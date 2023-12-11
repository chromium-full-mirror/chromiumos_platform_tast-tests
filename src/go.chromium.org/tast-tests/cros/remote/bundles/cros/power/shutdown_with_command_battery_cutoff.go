// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/security"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShutdownWithCommandBatteryCutoff,
		Desc:         "Verifies that system comes back after executing shutdown command with battery cutoff",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"timvp@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_ec"},
		Fixture:      fixture.NormalMode,
		Vars:         []string{"servo"},
		Timeout:      10 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.NoBatteryBootSupported()),
		Requirements: []string{"sys-fw-0022-v02"},
	})
}

func ShutdownWithCommandBatteryCutoff(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove watchdog for ccd: ", err)
	}

	// Make sure AC is attached so the DUT is powered after the battery is cut off.
	if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
		s.Fatal("Failed to set charger status to connected: ", err)
	}

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		testing.ContextLog(ctx, "Rebooting EC to restore battery connection")
		// Running ec command reboot from a powered off state (eg powerstate G3) still brings up DUT without additional powerkey press.
		if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
			s.Fatal("Failed to reboot EC: ", err)
		}

		testing.ContextLog(ctx, "Attempting to connect to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.DUT.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT after ec rebooot: ", err)
		}

	}(cleanupCtx)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	client := security.NewBootLockboxServiceClient(cl.Conn)
	if _, err := client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Disable battery sustainer - i.e. force charging
	testing.ContextLog(ctx, "Disable battery sustainer")
	if err := dut.Conn().CommandContext(ctx, "ectool", "chargecontrol", "normal").Run(); err != nil {
		s.Fatal("Failed to issue `ectool chargecontrol normal`: ", err)
	}

	// Cut off the battery to simulate booting without a battery.
	testing.ContextLog(ctx, "Cut off the battery")
	if err := dut.Conn().CommandContext(ctx, "ectool", "batterycutoff").Run(); err != nil {
		s.Fatal("Failed to issue `ectool batterycutoff`: ", err)
	}

	testing.ContextLog(ctx, "Shutting down the AP")
	powerOffCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Failed to execute shutdown command: ", err)
	}

	testing.ContextLog(ctx, "Waiting for AP to shutdown")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
		s.Fatal("Failed to wait for G3/S5 powerstate: ", err)
	}

	testing.ContextLog(ctx, "Pressing power key to turn on DUT")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		s.Fatal("Failed to press power key on DUT: ", err)
	}
	func() {
		testing.ContextLog(ctx, "Attempting to connect to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.DUT.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT after ec rebooot: ", err)
		}
	}()

	testing.ContextLog(ctx, "Logging into ChromeOS")
	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}

	// We expect 'err' to be non-nil (an error to be generated) since
	// 'ectool batterycutoff' was issued and the EC can no longer get good
	// values from the battery, including it's current charge status.
	_, err = h.Servo.GetBatteryChargeMAH(ctx)
	if err == nil {
		s.Fatal("Failed: battery is not cut off after power cycle")
	}
}
