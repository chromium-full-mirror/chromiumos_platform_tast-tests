// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	waitBootTimeout = testing.RegisterVarString(
		"platform.BootupTimesUSB.bootTime",
		"600",
		"Expected bootup time in seconds")
)

const (
	minBatteryLevel       = 30     // percentage
	chargerPortMinVoltage = 4400.0 // minimum expected voltage on the charger port on servo V4
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootupTimesUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures boot performance from USB",
		Contacts:     []string{"peep-fleet-infra-sw@google.com"},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		SoftwareDeps: []string{"crossystem"},
		ServiceDeps:  []string{"tast.cros.arc.PerfBootService", "tast.cros.platform.BootPerfService", "tast.cros.security.BootLockboxService"},
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name:    "usb_dev",
			Fixture: fixture.DevMode,
			Val:     fwCommon.BootModeUSBDev,
			Timeout: 60 * time.Minute,
		}, {
			Name:    "usb_recovery",
			Fixture: fixture.NormalMode,
			Val:     fwCommon.BootModeRecovery,
			Timeout: 60 * time.Minute,
		}},
	})
}

func BootupTimesUSB(ctx context.Context, s *testing.State) {
	bootToMode := s.Param().(fwCommon.BootMode)
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	bootTime, err := strconv.ParseInt(waitBootTimeout.Value(), 10, 32)
	if err != nil {
		s.Fatal("Failed to convert boot time: ", err)
	}
	s.Log("Boot Time for validation: ", bootTime)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}

	// Double-check that DUT starts in the right mode.
	curr, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Checking boot mode at beginning of test: ", err)
	}
	if curr != pv.BootMode {
		s.Logf("DUT started in boot mode %s. Setting up %s", curr, pv.BootMode)
		if err = ms.RebootToMode(ctx, pv.BootMode); err != nil {
			s.Fatalf("Failed to set up %s mode: %s", pv.BootMode, err)
		}
	}

	// Make sure correct image is on USB key
	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	switch bootToMode {
	case fwCommon.BootModeUSBDev:
		// Set GBB flags for dev boot from USB
		flags := fwpb.GBBFlagsState{
			Set:   []fwpb.GBBFlag{fwpb.GBBFlag_FORCE_DEV_BOOT_USB, fwpb.GBBFlag_FORCE_DEV_SWITCH_ON},
			Clear: []fwpb.GBBFlag{fwpb.GBBFlag_DEV_SCREEN_SHORT_DELAY},
		}
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, h.DUT, &flags); err != nil {
			s.Fatal("ClearAndSetGBBFlags for USB dev boot failed: ", err)
		}

		// Make USB visible to DUT and reset DUT
		s.Log("Enabling USB connection to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to USB mux to DUT: ", err)
		}
		s.Logf("Sleeping %s to let USB become visible to DUT", firmware.UsbVisibleTime)
		// GoBigSleepLint: It takes some time for usb mux state to take effect.
		if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
			s.Fatal("Failed to sleep and wait for usb mux state: ", err)
		}
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to reset dut by servo power reset: ", err)
		}

		// Cleanup.
		defer func(ctx context.Context) {
			s.Log("Performing clean up")

			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Error("Failed to reset dut by servo power reset: ", err)
			}
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
				s.Fatal("Failed to set USB mux to off: ", err)
			}
		}(ctx)

		s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Wait for firmware screen.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s (FirmwareScreen): %v", h.Config.FirmwareScreen, err)
		}

		// Sometimes, the sleep in waiting for the firmware screen to appear
		// might be too short for a few specific duts. Increase the number of
		// presses on ctrl_u to ensure that at least one of them is effective.
		for i := 0; i < 3; i++ {
			// Document ap states for debugging purposes.
			apPower, screenState, err := h.Servo.GetAPState(ctx)
			if err != nil {
				s.Log("Failed to get ap status: ", err)
			} else {
				s.Logf("Found ap %s and screen status %s", apPower, screenState)
			}
			s.Logf("Testing shortcuts %q", servo.CtrlU)
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
				s.Fatalf("Failed to press %s: %v", servo.CtrlU, err)
			}

			if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
				// Pressing space leads DUT to the confirmation page
				// for booting to normal mode, which helps bypass the
				// fw screen timeout, and ensures an extended stay.
				s.Log(ctx, "Pressing space key to bypass fw screen timeout")
				if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
					s.Fatal("Failed to press space: ", err)
				}
			}
			// GoBigSleepLint: Simulate a specific speed of button presses.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				s.Fatalf("Failed to sleep for %v second: %v", h.Config.KeypressDelay, err)
			}

			s.Log(ctx, "Pressing esc to return to the developer screen")
			if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
				s.Fatal("Failed to press esc key: ", err)
			}

			// GoBigSleepLint: Simulate a specific speed of button presses.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				s.Fatalf("Failed to sleep for %v second: %v", h.Config.KeypressDelay, err)
			}
		}
	case fwCommon.BootModeRecovery:
		// No need to set gbb flags because starting from normal boot already clears gbb flags
		// Setup power sink
		// We assume the device has enough battery (ie. >30%) to run the test to completion
		needBatterySink, err := recoveryRequirePDSink(ctx, h, s)
		if err != nil {
			s.Log("Will not set pd:snk for recovery mode: ", err)
		}

		// Setup cleanup
		defer func(ctx context.Context) {
			s.Log("Performing clean up")

			if needBatterySink {
				if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
					s.Log("Failed to restore pd:src: ", err)
				}
			}
			// GoBigSleepLint: Waiting 10 seconds for USB re-enumerate after PD role switch.
			testing.Sleep(ctx, 10*time.Second)
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
				s.Fatal("Failed to power off: ", err)
			}
			if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
				s.Log("Turn off USB drive on servo failed: ", err)
			}
			// GoBigSleepLint: Waiting 10 seconds before turning it on as the device can be still in transition to off.
			testing.Sleep(ctx, 10*time.Second)
			if h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				s.Fatal("Failed to power on: ", err)
			}
			s.Log("Boot in recovery mode: servo states recovered")
		}(ctx)

		// Perform reboot into recovery
		s.Log("Boot in Recovery Mode: starting")
		// Turn power off.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Boot in Recovery Mode: Power off: ", err)
		}
		// Next:Boot in recovery mode. The steps are:
		// Step 1. Switch the USB to DUT on the servo multiplexer
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Boot in Recovery Mode: Failed to set USB mux to DUT: ", err)
		}

		// Step 2. For servo V4, switch power delivery to sink mode. c.f.:
		// b/187900184.
		if needBatterySink {
			if err := h.Servo.SetPDRole(ctx, servo.PDRoleSnk); err != nil {
				s.Fatal("Boot in Recovery Mode: Failed to set pd:snk: ", err)
			}
		}
		// GoBigSleepLint: Sleep a few seconds to apply all previous states before boot in recovery mode.
		testing.Sleep(ctx, 1*time.Second)
		s.Log("Boot in Recovery Mode: Started try to boot in recovery mode by power_state:rec")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
			s.Fatal("Boot in Recovery Mode: Failure when trying to set power_state:rec with error: ", err)
		}
	default:
		s.Fatal("Error, unrecognized test case")
	}

	s.Log("Reconnecting to DUT")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, time.Duration(bootTime)*time.Second)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking that DUT has booted from removable device")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		s.Fatal("Failed to determine boot device type: ", err)
	}
	if !bootedFromRemovableDevice {
		s.Fatalf("DUT did not boot from the bootable device: got %v, want true", bootedFromRemovableDevice)
	}

	curr, err = h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Checking boot mode after reboot: ", err)
	}
	if curr != bootToMode {
		s.Fatalf("DUT did not boot in desired mode: got: %v, want %v", curr, bootToMode)
	}

	s.Logf("DUT rebooted in boot mode %s", curr)
}

func recoveryRequirePDSink(ctx context.Context, h *firmware.Helper, s *testing.State) (bool, error) {
	if _, err := h.CheckPowerSupplyDeviceStates(ctx); err != nil {
		return false, errors.Wrap(err, "could not determine battery state")
	}

	if connType, err := h.Servo.GetDUTConnectionType(ctx); err != nil {
		return false, errors.Wrap(err, "could not determine servo dut connection type")
	} else if connType != servo.DUTConnTypeC {
		s.Logf("Servo Supports Built-In PD Control: connection type %q does not match type-c", connType)
		return false, nil
	}

	if chgPortMV, err := h.Servo.GetFloat(ctx, servo.ChargerVoltageMV); err != nil {
		return false, errors.Wrap(err, "servo supports built in PD control")
	} else if chgPortMV < chargerPortMinVoltage {
		s.Logf("Servo Supports Built in PD Control: charger not plugged into servo V4, charger port voltage %f is less than the thresohld %f", chgPortMV, chargerPortMinVoltage)
		return false, nil
	} else {
		s.Logf("Servo Supports Built in PD Control: Charger port voltage %f is at least equal to the thresohld %f", chgPortMV, chargerPortMinVoltage)
	}

	return true, nil
}
