// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type devBootInternalMethod int

const (
	devBootInternalKeyboard devBootInternalMethod = iota
	devBootInternalMenu
	devBootInternalButton
)

type devBootInternalParams struct {
	enableDevBootUSB        bool
	defaultBootFromMainDisk bool
	setUpValidUSB           bool
	bootMethod              devBootInternalMethod
	expectedFwScreens       []fwCommon.FwScreenID
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootInternal,
		Desc: "Verify dev boot from internal disk works in various scenarios",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Name: "keyboard",
			Val: &devBootInternalParams{
				enableDevBootUSB:        false,
				defaultBootFromMainDisk: true,
				setUpValidUSB:           false,
				bootMethod:              devBootInternalKeyboard,
			},
			Timeout: 30 * time.Minute,
		}, {
			Name: "keyboard_with_usb",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: false,
				setUpValidUSB:           true,
				bootMethod:              devBootInternalKeyboard,
			},
			ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
			Timeout:          2 * time.Hour,
		}, {
			Name: "menu",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: false,
				setUpValidUSB:           true,
				bootMethod:              devBootInternalMenu,
			},
			ExtraTestBedDeps:  []string{tbdep.ServoUSBState("NORMAL")},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.LegacyMenuUI, hwdep.MenuUI)),
			Timeout:           2 * time.Hour,
		}, {
			Name: "button",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: false,
				setUpValidUSB:           true,
				bootMethod:              devBootInternalButton,
			},
			ExtraTestBedDeps:  []string{tbdep.ServoUSBState("NORMAL")},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
			Timeout:           2 * time.Hour,
		}, {
			Name: "insert_usb_screen",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: true,
				setUpValidUSB:           false,
				bootMethod:              devBootInternalKeyboard,
				expectedFwScreens:       []fwCommon.FwScreenID{fwCommon.DeveloperMode, fwCommon.DeveloperBootExternal},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI)),
			Timeout:           30 * time.Minute,
		}},
	})
}

func DevBootInternal(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testOpt := s.Param().(*devBootInternalParams)

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Error("Failed to ensure DUT has booted: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Error("Failed to disable dev boot from USB: ", err)
		}
		if err := h.SetDefaultBootDisk(ctx); err != nil {
			s.Error("Failed to set dev default boot target to disk: ", err)
		}
		if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
			s.Fatal("Failed to reboot with VT2 command: ", err)
		}

		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Error("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true

			// It could take a longer time to reconnect to the DUT.
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 5*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Error("Failed to reconnect to the DUT: ", err)
			}
		}
	}(cleanupCtx)

	if testOpt.enableDevBootUSB {
		if err := h.EnableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to enable usb boot: ", err)
		}
	} else {
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to disable usb boot: ", err)
		}
	}
	if !testOpt.defaultBootFromMainDisk {
		if err := h.SetDefaultBootUSB(ctx); err != nil {
			s.Fatal("Failed to set default boot target to the usb: ", err)
		}
	}

	if testOpt.setUpValidUSB {
		s.Log("Setup USB key")
		skipFlashUSB := false
		if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
			var err error
			skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
			if err != nil {
				s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
			}
		}
		var cs *testing.CloudStorage
		if !skipFlashUSB {
			cs = s.CloudStorage()
		}
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBKey not working: ", err)
		}

		s.Log("Inserting a valid USB to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to insert USB to DUT: ", err)
		}

		// GoBigSleepLint: It may take some time for usb mux state to
		// take effect.
		if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
			s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
		}
	} else {
		s.Log("Unplugging USB from DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
			s.Fatal("Failed to unplug USB from DUT: ", err)
		}
	}

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func(ctx context.Context) {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}(cleanupCtx)
	}

	s.Log("Rebooting DUT by warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset DUT: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable after sending a warm reset: ", err)
	}

	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
			s.Error("Failed to detect firmware screen: ", err)
		}
	} else {
		s.Logf("Resetting firmware screen timeout for %s (FirmwareScreen)", h.Config.FirmwareScreen)
		endTime := time.Now().Add(h.Config.FirmwareScreen)
		for time.Now().Before(endTime) {
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				s.Fatal("Failed to press space key: ", err)
			}
			// On KeyboardDevSwitcher machines, pressing space triggers the
			// to_norm screen. Revert to the developer screen with the
			// esc key.
			if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
				// GoBigSleepLint: Sleep for model specific time.
				if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
					s.Fatalf("Failed to sleep for %s (KeypressDelay): %v", h.Config.KeypressDelay, err)
				}
				if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
					s.Fatal("Failed to press esc: ", err)
				}
			}
			// GoBigSleepLint: Avoid hitting space too fast.
			if err := testing.Sleep(ctx, 2*time.Second); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}
	}

	if err := h.ByPassDevBootTimeout(ctx); err != nil {
		s.Fatal("Failed to bypass dev boot timeout: ", err)
	}

	if state.RemoveServoChargerRequired && state.IsServoChargerConnected {
		s.Log("Removing servo charger")
		if err := h.SetDUTPower(ctx, false); err != nil {
			s.Fatal("Failed to remove charger: ", err)
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}

	if err := h.ReturnToDeveloperScreen(ctx); err != nil {
		s.Fatal("Failed to return to developer screen: ", err)
	}

	if testOpt.enableDevBootUSB && !testOpt.setUpValidUSB {
		// Verify that Ctrl-U doesn't boot up the DUT from the USB.
		testing.ContextLog(ctx, "Pressing Ctrl-U")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatal("Failed to press Ctrl-U: ", err)
		}
		if h.HasAPFwState {
			if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperBootExternal); err != nil {
				s.Error("Failed to detect firmware screen: ", err)
			}
		}
		// GoBigSleepLint: Simulate a specific speed of key press.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %v s", h.Config.KeypressDelay)
		}
	}

	if err := bootupFromDevScreen(ctx, h, testOpt.bootMethod); err != nil {
		s.Fatal("DUT failed to boot from dev screen: ", err)
	}

	// GoBigSleepLint: Simulate a specific speed of key and button.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		s.Fatalf("Failed to sleep for %v s", h.Config.KeypressDelay)
	}

	// If presses from bootupFromDevScreen were ineffective, ensure that DUT
	// did not boot because of the dev screen timeout by continuously pressing
	// space key here that would have stopped the DUT from booting. Only do so if
	// the default boot target was set to the main disk. If the target was set
	// to USB, we should be able to catch it later in checking for the boot mode.
	if testOpt.defaultBootFromMainDisk {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			// Pressing space leads keyboardDevSwitcher devices to the to-norm screen,
			// and leaves menuSwitcher and tabletDetachableSwitcher on the dev screen.
			// In either case, the DUT would not boot up.
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press space while waiting for DUT to connect")
			}
			// GoBigSleepLint: Simulate a specific speed of key press.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "failed to sleep for %v s", h.Config.KeypressDelay)
			}
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 25*time.Second)
			defer cancelWaitConnect()

			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: h.Config.DelayRebootToPing}); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	} else {
		// Since we might have booted from USB incorrectly, wait the USB time.
		s.Logf("Waiting for SSH for %v (USBImageBootTimeout)", h.Config.USBImageBootTimeout)
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}

	s.Log("Checking for DUT in dev mode")
	bootFromDevMode, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeDev)
	if err != nil {
		s.Fatal("Failed to get DUT boot mode: ", err)
	}
	if !bootFromDevMode {
		s.Fatal("DUT did not boot to dev mode as expected")
	}

	if testOpt.expectedFwScreens != nil {
		found, err := h.Reporter.CheckDisplayedScreens(ctx, testOpt.expectedFwScreens)
		if err != nil {
			s.Fatal("Failed to check displayed screens: ", err)
		}
		if !found {
			s.Fatal("Did not find firmware screens as expected")
		}
	}
}

// bootupFromDevScreen attempts some combination of key, or button presses
// to boot up the DUT from dev screen.
func bootupFromDevScreen(ctx context.Context, h *firmware.Helper, bootMethod devBootInternalMethod) error {
	switch bootMethod {
	case devBootInternalKeyboard:
		testing.ContextLog(ctx, "Pressing Ctrl-D")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press Ctrl-D")
		}
	case devBootInternalMenu:
		menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
		if err != nil {
			return errors.Wrap(err, "creating new menuBypasser")
		}
		if err := menuBypasser.BypassDevMode(ctx); err != nil {
			return errors.Wrap(err, "failed to bypass dev mode")
		}
	case devBootInternalButton:
		testing.ContextLog(ctx, "Holding volumn down for 5 second")
		if err := h.Servo.SetInt(ctx, servo.VolumeDownHold, 5000); err != nil {
			return errors.Wrap(err, "failed to press and hold volumn down")
		}
	default:
		return errors.New("unrecongized booting method")
	}
	return nil
}
