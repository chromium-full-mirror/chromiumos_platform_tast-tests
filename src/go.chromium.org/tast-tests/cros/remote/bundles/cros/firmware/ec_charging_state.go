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
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Create enum to specify which tests need to be run
type ecChargingTest int

const (
	acStateDuringSuspend ecChargingTest = iota
	voltageOnDischarge
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
		Attr:         []string{"group:firmware", "firmware_unstable"},
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
			Name:    "ac_on_suspend",
			Timeout: 5 * time.Minute,
			Val:     acStateDuringSuspend,
		}, {
			Name:    "discharge",
			Timeout: 70 * time.Minute,
			Val:     voltageOnDischarge,
		}, {
			Name:    "full_charge",
			Timeout: 70 * time.Minute,
			Val:     statusOnFullCharge,
		},
		},
	})
}

const (
	fullBatteryPercent     = 95.0
	targetDischargePercent = 93.0
	fullChargePollTimeout  = 60 * time.Minute
	dischargePollTimeout   = 60 * time.Minute
	chargePollInterval     = 1 * time.Second
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
	case acStateDuringSuspend:
		// ----------- Test #1: Check removal of power --------
		s.Log("Start out with power attached, suspending the DUT, and remove power")
		if err := testDisconnectChargerAfterSuspend(ctx, h); err != nil {
			s.Fatal("Failed disconnect ac after suspend test: ", err)
		}
		// ----------- Test #2: Check addition of power while asleep -----------
		s.Log("With power removed, suspend the DUT and then add power")
		if err := testConnectChargerAfterSuspend(ctx, h); err != nil {
			s.Fatal("Failed connect ac after suspend test: ", err)
		}
	case voltageOnDischarge:
		// ----------- Test #3: Check discharges the DUT then checks its voltages -----------
		// Disable Charge Limit for this test, since it messes with detecting if the
		// battery is correctly charging while plugged in.
		if err := testChargingVoltagesAfterDischarge(ctx, h); err != nil {
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

func testConnectChargerAfterSuspend(ctx context.Context, h *firmware.Helper) error {
	if err := firmware.PollToSetChargerStatus(ctx, h, false); err != nil {
		return errors.Wrap(err, "failed to set charger status to disconnected")
	}

	if err := suspendDUTAndCheckCharger(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to suspend DUT and check if charger is attached")
	}

	var battery *firmware.ECBatteryState
	var err error

	testing.ContextLog(ctx, "Poll for expected battery state")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		battery, err = firmware.GetECBatteryStatus(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get current battery status")
		}
		// Don't check for discharging state, fully charged occasionally also sets discharging state.
		if battery.StatusCode&firmware.ECFullyCharged == 0 && battery.Charging != "Not Allowed" && battery.StatusCode&firmware.ECDischarging != 0 {
			return errors.Errorf("incorrect battery state, expected Charging/Fully Charged, got status: %v", battery.Status)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to poll for fully charged battery level in DUT")
	}

	if err := compareKernelAndECBatteryStatus(ctx, h, battery); err != nil {
		return errors.Wrap(err, "kernel and EC battery state mismatch")
	}

	return nil
}

func testDisconnectChargerAfterSuspend(ctx context.Context, h *firmware.Helper) error {
	if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to set charger status to connected")
	}

	if err := suspendDUTAndCheckCharger(ctx, h, false); err != nil {
		return errors.Wrap(err, "failed to suspend DUT and check if charger is attached")
	}

	var battery *firmware.ECBatteryState
	var err error

	testing.ContextLog(ctx, "Poll for expected battery state")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		battery, err = firmware.GetECBatteryStatus(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get current battery status")
		}
		if battery.StatusCode&alarmMask != 0 {
			return errors.Errorf("battery threw unexpected alarms %v, had statuses: %v", battery.Alarms, battery.Status)
		}
		if (battery.StatusCode & (firmware.ECTerminateChargeAlarm | firmware.ECFullyCharged)) == firmware.ECTerminateChargeAlarm {
			return errors.Errorf("battery raising terminate charge alarm non-full, status: %v", battery.Status)
		}
		if battery.StatusCode&firmware.ECDischarging == 0 {
			return errors.Errorf("incorrect battery state, expected discharging, got status: %v", battery.Status)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to charge battery")
	}

	if err := compareKernelAndECBatteryStatus(ctx, h, battery); err != nil {
		return errors.Wrap(err, "kernel and EC battery state mismatch")
	}

	return nil
}

func testChargingVoltagesAfterDischarge(ctx context.Context, h *firmware.Helper) error {
	battery, err := firmware.GetECBatteryStatus(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get ec battery state")
	}

	if battery.Charge > targetDischargePercent {
		testing.ContextLog(ctx, "Disconnecting charger")
		if err := firmware.PollToSetChargerStatus(ctx, h, false); err != nil {
			return errors.Wrap(err, "failed to disconnect charger")
		}

		// As the firmware test with bootModeNormal does not receive
		// browser services on its initialization, we cannot easily
		// use Chrome for battery drain procedure. Instead, we can
		// simply spawn stress-ng (which seems to be available in
		// base rootfs) for specified amount of time.
		// See also battery_service.go:DrainBattery

		script := "cd /usr/local/bin; stress-ng --cpu 32 --timeout 1m"

		testing.ContextLog(ctx, "Initiating battery discharging")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			testing.ContextLog(ctx, "Stressing CPU to discharge battery")
			cmd := h.DUT.Conn().CommandContext(ctx, "sh", "-c", script)
			if out, err := cmd.Output(ssh.DumpLogOnError); err != nil {
				err := errors.Wrapf(err, "failed to discharge battery using CPU stress, got output: %v", string(out))
				testing.PollBreak(err)
			}

			battery, err := firmware.GetECBatteryStatus(ctx, h)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get battery state: ", err)
				return errors.Wrap(err, "failed to get battery state")
			}
			if battery.Charge > targetDischargePercent {
				testing.ContextLogf(ctx, "Current charge: %v, target: %v", battery.Charge, targetDischargePercent)
				return errors.Errorf("Not enough battery discharged: %v, want %v", battery.Charge, targetDischargePercent)
			}

			return nil
			// poll at 1s since the stress script will block progress for 1 minute
		}, &testing.PollOptions{Timeout: dischargePollTimeout, Interval: chargePollInterval}); err != nil {
			return errors.Wrap(err, "failed to discharge battery")
		}
	}

	testing.ContextLog(ctx, "Reconnecting charger")
	if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
		return errors.Wrap(err, "failed to connect charger")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := firmware.CheckChargingState(ctx, h); err != nil {
			return errors.Wrap(err, "failed to verify expected charging voltages")
		}

		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
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

	kernelBatteryState, err := firmware.GetKernelBatteryState(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get kernel battery state")
	}

	testing.ContextLog(ctx, "Kernel reported: ", kernelBatteryState)
	if kernelBatteryState == firmware.KernelCharging {
		return errors.Errorf("the EC reported Fully Charged state but kernel reported %v, expected fully charged/discharging/not charging instead", kernelBatteryState)
	}

	return nil
}

func compareKernelAndECBatteryStatus(ctx context.Context, h *firmware.Helper, battery *firmware.ECBatteryState) error {
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
			return errors.Errorf("Kernel reports battery status to be charging, but ec status was %v instead (expect discharging)", battery.Status)
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
