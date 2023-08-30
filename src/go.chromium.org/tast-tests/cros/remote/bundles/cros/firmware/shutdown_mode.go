// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type shutdownModeTestParams struct {
	shutdownmode string
}

const (
	powerButton string = "powerbutton"
	powerOff    string = "poweroff"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ShutdownMode, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Verifies that system comes back after power button press and poweroff",
		Contacts: []string{
			"chromeos-faft@google.com",
			"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.ui.ScreenLockService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		Vars:         []string{"servo"},
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "group:intel-nda"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		Params: []testing.Param{{
			Name: "power_button",
			Val:  shutdownModeTestParams{shutdownmode: powerButton},
		}, {
			Name: "poweroff_command",
			Val:  shutdownModeTestParams{shutdownmode: powerOff},
		},
		},
	})
}

func ShutdownMode(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	testOpt := s.Param().(shutdownModeTestParams)
	const (
		cmdTimeout             = 3 * time.Second // cmdTimeout is a short duration used for sending commands.
		powerStateInterval     = 1 * time.Second
		powerStateTimeout      = 30 * time.Second
		expectedPrevSleepState = 5 // expectedPrevSleepState is the expected previous sleep state value for coldboot.
	)
	// Logging into chrome.
	chromeLogin := func() {
		s.Log("Login to Chrome")
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)
		screenLockService := ui.NewScreenLockServiceClient(cl.Conn)
		if _, err := screenLockService.NewChrome(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to login chrome: ", err)
		}
		defer screenLockService.CloseChrome(ctx, &empty.Empty{})
	}
	chromeLogin()
	pwrOnDUT := func() {
		s.Log("Power Normal Pressing")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			s.Fatal("Failed to power button press: ", err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Log("Failed to wake up DUT. Retrying")
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
				s.Fatal("Failed to power button press: ", err)
			}
			if err := dut.WaitConnect(waitCtx); err != nil {
				s.Fatal("Failed to wait connect DUT: ", err)
			}
		}
	}
	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			pwrOnDUT()
		}
	}(ctx)
	if testOpt.shutdownmode == "powerbutton" {
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOff)); err != nil {
			s.Fatal("Failed to power button press: ", err)
		}
		if err := h.WaitForPowerStates(ctx, powerStateInterval, powerStateTimeout, "G3"); err != nil {
			s.Fatal("Failed to get G3 powerstate: ", err)
		}
		pwrOnDUT()
		chromeLogin()
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
			s.Fatalf("Failed Previous Sleep state is not %d after powerbutton long pressing via servo: %v", expectedPrevSleepState, err)
		}
	}
	if testOpt.shutdownmode == "poweroff" {
		powerOffCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
		defer cancel()
		if err := h.DUT.Conn().CommandContext(powerOffCtx, "poweroff").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			s.Fatal("Failed to power off DUT: ", err)
		}
		if err := h.WaitForPowerStates(ctx, powerStateInterval, powerStateTimeout, "G3"); err != nil {
			s.Fatal("Failed to get G3 powerstate: ", err)
		}
		pwrOnDUT()
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
			s.Fatalf("Failed Previous Sleep state is not %d after executing poweroff command: %v", expectedPrevSleepState, err)
		}
		haltCtx, cancel := context.WithTimeout(ctx, cmdTimeout)
		defer cancel()
		if err := h.DUT.Conn().CommandContext(haltCtx, "halt").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			s.Fatal("Failed to halt DUT: ", err)
		}
		if err := h.WaitForPowerStates(ctx, powerStateInterval, powerStateTimeout, "G3"); err != nil {
			s.Fatal("Failed to get G3 powerstate: ", err)
		}
		pwrOnDUT()
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
			s.Fatalf("Failed Previous Sleep state is not %d after executing halt command: %v", expectedPrevSleepState, err)
		}
	}
}
