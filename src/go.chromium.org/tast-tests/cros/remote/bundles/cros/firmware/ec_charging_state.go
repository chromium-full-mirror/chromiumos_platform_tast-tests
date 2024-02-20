// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Create enum to specify which tests need to be run
type ecChargingTest int

const (
	voltageOnDischarge ecChargingTest = iota
	statusOnFullCharge
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECChargingState,
		Desc: "Check charging state changes are captured when DUT is asleep",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_ccd", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.PowerMenuService",
			"tast.cros.browser.ChromeService",
			"tast.cros.power.BatteryService",
		},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name:    "discharge",
			Timeout: 70 * time.Minute,
			Val:     voltageOnDischarge,
		}, {
			Name:    "full_charge",
			Timeout: 120 * time.Minute,
			Val:     statusOnFullCharge,
		},
		},
	})
}

const (
	fullBatteryPercent     = 95.0
	targetDischargePercent = 90.0
	fullChargePollTimeout  = 110 * time.Minute
	// alarmMask is a mask ignoring expected battery alarms like terminate charge and over charged.
	alarmMask = (0xFF00 & ^firmware.ECTerminateChargeAlarm & ^firmware.ECOverChargedAlarm)
)

func ECChargingState(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove watchdog for ccd: ", err)
	}

	if out, err := h.Servo.RunECCommandGetOutputNoConsoleLogs(ctx, "dsleep", []string{`timeout:\s+(\d+)\s*sec`}); err == nil {
		setLowPowerIdleDelay := out[0][1]
		s.Log("Original dlseep timeout: ", setLowPowerIdleDelay)
		s.Log("Setting dsleep to 20s")
		if err := h.Servo.RunECCommand(ctx, "dsleep 20"); err != nil {
			s.Fatal("Failed to set dlseep to 20: ", err)
		}
		defer func() {
			s.Log("Setting dsleep back to original value of ", setLowPowerIdleDelay)
			if err := h.Servo.RunECCommand(ctx, "dsleep "+setLowPowerIdleDelay); err != nil {
				s.Fatalf("Failed to set dlseep to %s: %v", setLowPowerIdleDelay, err)
			}
		}()
	} else {
		s.Log("Unable to set dsleep")
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to RPC: ", err)
	}
	client := power.NewBatteryServiceClient(h.RPCClient.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start BatteryServiceClient: ", err)
	}
	defer client.Close(ctx, &empty.Empty{})
	if _, err := client.StopChargeLimit(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop charge limit: ", err)
	}

	defer func() {
		s.Log("Reconnect to charger on test end")
		if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
			s.Fatal("Failed to set charger status to connected: ", err)
		}
	}()

	switch s.Param().(ecChargingTest) {
	case voltageOnDischarge:
		// ----------- Test #3: Check discharges the DUT then checks its voltages -----------
		// Disable Charge Limit for this test, since it messes with detecting if the
		// battery is correctly charging while plugged in.
		if err := firmware.TestChargingVoltagesAfterDischarge(ctx, h, targetDischargePercent); err != nil {
			s.Fatal("Failed checking voltages after discharge test: ", err)
		}
	case statusOnFullCharge:
		// ----------- Test #4: Check EC reports expected alarms/status at full charge -----------	//
		// See b/151181037.
		if err := testFullChargeAlarm(ctx, h); err != nil {
			s.Fatal("Failed checking battery status after full charge test: ", err)
		}
	}
}

func testFullChargeAlarm(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Connect charger")
	if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to connect charger")
	}

	testing.ContextLogf(ctx, "Wait for DUT to reach fully charged state up to %s minutes", fullChargePollTimeout)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		battery, err := firmware.GetECBatteryStatus(ctx, h)
		if err != nil {
			return errors.Wrap(err, "error getting battery status")
		}
		testing.ContextLogf(ctx, "Current charge: %v, status: %v", battery.Charge, battery.Status)
		if battery.StatusCode&firmware.ECFullyCharged == 0 {
			return errors.Errorf("expected DUT to be fully charged, actual charge level was: %v, and status was: %v", battery.Charge, battery.Status)
		}
		return nil
	}, &testing.PollOptions{Timeout: fullChargePollTimeout, Interval: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to poll for fully charged battery level in DUT")
	}

	testing.ContextLog(ctx, "Wait for kernel to report fully charged")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		kernelBatteryState, err := firmware.GetKernelBatteryState(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get kernel battery state")
		}

		testing.ContextLog(ctx, "Kernel reported: ", kernelBatteryState)
		if kernelBatteryState == firmware.KernelCharging {
			return errors.Errorf("the EC reported Fully Charged state but kernel reported %v, expected fully charged/discharging/not charging instead", kernelBatteryState)
		}

		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to poll for fully charged state from kernel")
	}
	return nil
}

func compareKernelAndECBatteryStatus(ctx context.Context, h *firmware.Helper, battery *firmware.ECBatteryState) error {
	testing.ContextLog(ctx, "Verify kernel reports same state as EC")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		kernelBatteryState, err := firmware.GetKernelBatteryState(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get kernel battery state")
		}

		switch kernelBatteryState {
		case firmware.KernelFullyCharged:
			if battery.StatusCode&firmware.ECFullyCharged == 0 && battery.Display < fullBatteryPercent {
				return errors.Errorf("Kernel reports battery status to be fully charged, but ec status was %v instead (expect fully charged or not charging)", battery.Status)
			}
			return nil
		case firmware.KernelCharging:
			if battery.StatusCode&firmware.ECDischarging != 0 {
				return errors.Errorf("Kernel reports battery status to be charging, but ec status was %v instead (expect charging)", battery.Status)
			}
			return nil
		case firmware.KernelDischarging, firmware.KernelNotCharging:
			// EC might report the battery is not discharging if its fully charged, so raise error only if not discharging and not fully charged.
			if battery.StatusCode&firmware.ECDischarging == 0 && battery.StatusCode&firmware.ECFullyCharged == 0 && battery.Display < fullBatteryPercent {
				return errors.Errorf("Kernel reports battery status to be discharging, but ec status was %v instead (expect discharging and/or full charged)", battery.Status)
			}
			return nil
		default:
			return errors.Errorf("unexpected battery state %q from kernel", kernelBatteryState)
		}
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to poll for matching ec and kernel battery state")
	}
	return nil
}

func suspendDUTAndCheckCharger(ctx context.Context, h *firmware.Helper, expectChargerAttached bool) error {
	testing.ContextLog(ctx, "Suspending DUT")

	if err := h.DUT.Conn().CommandContext(ctx, "pgrep", "powerd").Run(); err != nil {
		return errors.Wrap(err, "powerd is not running. Need that to run suspend cmds")
	}

	// Use Start because run will never return (we are suspending).
	cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=3")
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	testing.ContextLog(ctx, "Checking for S0ix or S3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0ix", "S3"); err != nil {
		return errors.Wrap(err, "failed to get S0ix or S3 powerstate")
	}

	testing.ContextLog(ctx, "Setting charger connected to ", expectChargerAttached)
	if err := firmware.PollToSetChargerStatus(ctx, h, expectChargerAttached); err != nil {
		return errors.Wrap(err, "failed to set charger status")
	}

	testing.ContextLog(ctx, "Power on DUT with power key")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press power key on DUT")
	}

	testing.ContextLog(ctx, "Wait for DUT to reach S0 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "DUT failed to reach S0 after power button pressed")
	}

	testing.ContextLog(ctx, "Wait for DUT to connect")
	if err := h.WaitConnect(ctx, firmware.FromHibernation, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to connect to DUT")
	}

	return nil
}
