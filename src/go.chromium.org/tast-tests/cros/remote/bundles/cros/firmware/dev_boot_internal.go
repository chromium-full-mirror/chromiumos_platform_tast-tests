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
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
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
			Timeout: 15 * time.Minute,
		}, {
			Name: "keyboard_with_usb",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: false,
				setUpValidUSB:           true,
				bootMethod:              devBootInternalKeyboard,
			},
			ExtraAttr: []string{"firmware_usb"},
			Timeout:   2 * time.Hour,
		}, {
			Name: "menu",
			Val: &devBootInternalParams{
				enableDevBootUSB:        true,
				defaultBootFromMainDisk: false,
				setUpValidUSB:           true,
				bootMethod:              devBootInternalMenu,
			},
			ExtraAttr:         []string{"firmware_usb"},
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
			ExtraAttr:         []string{"firmware_usb"},
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
			ExtraAttr:         []string{"firmware_usb"},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI)),
			Timeout:           15 * time.Minute,
		}},
	})
}

func DevBootInternal(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testOpt := s.Param().(*devBootInternalParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure dut has booted: ", err)
		}
		if err := h.DisableDevBootUSB(ctx); err != nil {
			s.Fatal("Failed to disable dev boot from USB: ", err)
		}
		if err := h.SetDefaultBootDisk(ctx); err != nil {
			s.Fatal("Failed to set dev default boot target to disk: ", err)
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

	s.Log("Rebooting dut by warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset dut: ", err)
	}

	s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	if testOpt.enableDevBootUSB && !testOpt.setUpValidUSB {
		// Verify that Ctrl-U doesn't boot up the DUT from the USB.
		testing.ContextLog(ctx, "Pressing Ctrl-U")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatal("Failed to press Ctrl-U: ", err)
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
				return errors.Wrap(err, "failed to press space while waiting for dut to connect")
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
			s.Fatal("Failed to reconnect to dut: ", err)
		}
	} else {
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to dut: ", err)
		}
	}

	s.Log("Checking for DUT in dev mode")
	bootFromDevMode, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeDev)
	if err != nil {
		s.Fatal("Failed to get dut boot mode: ", err)
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
