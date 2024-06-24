// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"bufio"
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BatteryChargeDuringShutdown,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies battery is charging during DUT shutdown",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps:  []string{"tast.cros.power.BatteryService"},
		VarDeps:      []string{"servo"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Attr:         []string{"group:intel-stress"},
		Fixture:      fixture.NormalMode,
		Timeout:      1 * time.Hour,
	})
}

// BatteryChargeDuringShutdown verifies battery charging is happening
// or not during DUT shutdown state.
func BatteryChargeDuringShutdown(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	dut := s.DUT()
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	origPdRole, err := h.Servo.GetPDRole(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve original USB PD role for Servo: ", err)
	}
	if origPdRole == servo.PDRoleNA {
		s.Fatal("Test requires Servo V4 or never to for operating DUT power delivery role through servo_pd_role")
	}

	cl, err := rpc.Dial(ctx, h.DUT, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	client := power.NewBatteryServiceClient(cl.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer client.Close(cleanupCtx, &empty.Empty{})

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := firmware.BootDutViaPowerPress(ctx, h, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
		s.Log("Getting back to original USB PD role")
		if err := h.Servo.SetPDRole(ctx, origPdRole); err != nil {
			s.Fatal("Failed to get back to original USB PD role: ", err)
		}
	}(cleanupCtx)

	s.Log("Stopping power supply")
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to stop power supply: ", err)
	}

	// Read battery info before initially.
	initbatteryCharge, err := batteryPercentage(ctx, dut)
	if err != nil {
		s.Fatal("Failed to read battery info before shutdown: ", err)
	}

	testing.ContextLogf(ctx, "initial battery charge: %f", initbatteryCharge)
	const expectedBatterypercent float64 = 75
	if initbatteryCharge >= expectedBatterypercent {
		testing.ContextLogf(ctx, "Battery percentage is %f more than expected %f battery pecentage, since draining battery", initbatteryCharge, expectedBatterypercent)
		request := power.BatteryRequest{MinPercentage: 69, MaxPercentage: 70}
		if _, err := client.PrepareBattery(ctx, &request); err != nil {
			s.Fatal("Failed to drain battery: ", err)
		}
	}

	// Read battery info before shutdown with charger unplugged.
	batteryPercentBeforeShutdown, err := batteryPercentage(ctx, dut)
	if err != nil {
		s.Fatal("Failed to read battery info before shutdown: ", err)
	}

	// Performing DUT Shutdown.
	if err := dut.Conn().CommandContext(ctx, "shutdown", "-h", "now").Run(); err != nil {
		s.Fatal("Failed to execute shutdown command: ", err)
	}
	sdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed to wait for unreachable: ", err)
	}

	// During DUT shutdown state plug power supply via Servo-V4
	s.Log("Starting power supply after shutdown")
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Fatal("Failed to plug power supply via Servo-V4: ", err)
	}

	s.Log("wait for 10 minutes")
	// Checking whether battery is able to charge in shutdown state.
	waitfor10mins, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := dut.WaitConnect(waitfor10mins); err == nil {
		s.Fatal("Failed to Wait for 10 min DUT in shoutdown: ", err)
	}

	// Power on DUT after performing shutdown.
	if err := firmware.BootDutViaPowerPress(ctx, h, dut); err != nil {
		s.Fatal("Failed to power on DUT: ", err)
	}

	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to unplug power supply via Servo-V4 during shutdown: ", err)
	}

	// Read battery info after shutdown with charger unplugged.
	batteryPercentAfterShutdown, err := batteryPercentage(ctx, dut)
	if err != nil {
		s.Fatal("Failed to read battery info after shutdown: ", err)
	}

	if batteryPercentAfterShutdown < batteryPercentBeforeShutdown {
		s.Fatal("Failed to charge DUT during shutdown")
	}
}

// batteryPercentage returns battery percentage info of DUT.
func batteryPercentage(ctx context.Context, dut *dut.DUT) (float64, error) {
	out, err := dut.Conn().CommandContext(ctx, "power_supply_info").Output()
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to get power supply info")
	}
	var matches []string
	batteryPercentRe := regexp.MustCompile(`^\s*percentage:\s+([0-9.]+)`)
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		match := batteryPercentRe.FindStringSubmatch(sc.Text())
		if match == nil {
			continue
		}
		matches = append(matches, match[1])
	}

	if len(matches) < 1 {
		return 0.0, errors.Wrap(err, "failed to find battery percent value")
	}
	batteryPercent := matches[0]
	curBatteryPercent, err := strconv.ParseFloat(batteryPercent, 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to convert from string to float")
	}
	return curBatteryPercent, nil
}
