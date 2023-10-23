// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
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

const (
	fromUSB = iota
	fromInternal
)

type usbState struct {
	isCorrupted bool
}

type devBootInvalidUSBParams struct {
	validBootAfterInvalidUSB int
	expectedFwScreens        map[firmware.ModeSwitcherType][]fwCommon.FwScreenID
	expectedBootMode         fwCommon.BootMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootInvalidUSB,
		Desc: "Verify boot from invalid usb is not possible, and check the corresponding firmware screens displayed",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Name:    "restore_usb",
			Timeout: 2 * time.Hour,
			Val: &devBootInvalidUSBParams{
				validBootAfterInvalidUSB: fromUSB,
				expectedFwScreens: map[firmware.ModeSwitcherType][]fwCommon.FwScreenID{
					firmware.KeyboardDevSwitcher: []fwCommon.FwScreenID{
						fwCommon.LegacyDeveloperWarning,
						// Screen flashes on seeing invalid USB.
						fwCommon.LegacyBlank,
						fwCommon.LegacyDeveloperWarning,
						// Go to TO_NORM screen to bypass the timeout.
						fwCommon.LegacyDeveloperToNorm,
						fwCommon.LegacyDeveloperWarning,
						// Boot from USB.
						fwCommon.LegacyBlank,
					},
					firmware.TabletDetachableSwitcher: []fwCommon.FwScreenID{
						fwCommon.LegacyDeveloperWarningMenu,
						// Screen flashes on seeing invalid USB.
						fwCommon.LegacyBlank,
						fwCommon.LegacyDeveloperWarningMenu,
						// Boot from USB.
						fwCommon.LegacyBlank,
					},
					firmware.MenuSwitcher: []fwCommon.FwScreenID{
						fwCommon.DeveloperMode,
						fwCommon.DeveloperInvalidDisk,
						fwCommon.DeveloperBootExternal,
					},
				},
				expectedBootMode: fwCommon.BootModeUSBDev,
			},
		}, {
			Name:    "back",
			Timeout: 10 * time.Minute,
			Val: &devBootInvalidUSBParams{
				validBootAfterInvalidUSB: fromInternal,
				expectedFwScreens: map[firmware.ModeSwitcherType][]fwCommon.FwScreenID{
					firmware.MenuSwitcher: []fwCommon.FwScreenID{
						fwCommon.DeveloperMode,
						fwCommon.DeveloperInvalidDisk,
						fwCommon.DeveloperMode,
					},
				},
				expectedBootMode: fwCommon.BootModeDev,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.MenuUI)),
		},
		},
	})
}

func DevBootInvalidUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testOpt := s.Param().(*devBootInvalidUSBParams)
	s.Log("Setting up the USB key")
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
	curUSBState := usbState{isCorrupted: false}

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
		if err := curUSBState.restoreUSB(ctx, h); err != nil {
			s.Fatal("Failed to restore USB: ", err)
		}
	}(cleanupCtx)

	if err := h.EnableDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to enable usb boot: ", err)
	}

	if err := curUSBState.insertInvalidUSB(ctx, h); err != nil {
		s.Fatal("Failed to insert invalid USB: ", err)
	}

	s.Log("Rebooting the DUT with warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to reboot the DUT with warm reset: ", err)
	}

	s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Wait for firmware screen.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s (FirmwareScreen): %v", h.Config.FirmwareScreen, err)
	}

	var connectionTimeout time.Duration
	var devScreenBootSteps []func(ctx context.Context, h *firmware.Helper) error
	switch testOpt.validBootAfterInvalidUSB {
	case fromUSB:
		devScreenBootSteps = []func(ctx context.Context, h *firmware.Helper) error{
			ctrlUBootFromUSB,
			curUSBState.restoreUSBOnFirmwareScreen,
		}
		if h.Config.ModeSwitcherType != firmware.MenuSwitcher {
			devScreenBootSteps = append(devScreenBootSteps, ctrlUBootFromUSB)
		}
		connectionTimeout = h.Config.USBImageBootTimeout
	case fromInternal:
		devScreenBootSteps = []func(ctx context.Context, h *firmware.Helper) error{
			menuUIBootFromUSB,
			selectBack,
			ctrlDBootFromInternal,
		}
		connectionTimeout = h.Config.DelayRebootToPing
	default:
		s.Fatal("Unrecongized boot steps")
	}

	for _, step := range devScreenBootSteps {
		if err := step(ctx, h); err != nil {
			s.Fatal("Failed to run step: ", err)
		}
		// GoBigSleepLint: Simulate a specific speed of key presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %v second", h.Config.KeypressDelay)
		}
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, connectionTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	expectedBootMode, err := h.Reporter.CheckBootMode(ctx, testOpt.expectedBootMode)
	if err != nil {
		s.Fatal("Failed to check dut boot mode: ", err)
	}
	if !expectedBootMode {
		s.Fatalf("Failed to boot from expected boot mode, expected mode: %s", testOpt.expectedBootMode)
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
			if err := h.SaveCBMEMLogs(ctx, saveLogPath); err != nil {
				s.Fatal("Failed to save firmware log: ", err)
			}
		}
	}(cleanupCtx)

	match, err := h.Reporter.CheckDisplayedScreens(ctx, testOpt.expectedFwScreens[h.Config.ModeSwitcherType])
	if err != nil {
		s.Fatal("Failed to check displayed firmware screens: ", err)
	}
	if !match {
		s.Fatal("Did not find firmware screens as expected")
	}
}

func (usb *usbState) insertInvalidUSB(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Setting up an invalid USB")
	usbdev, err := h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
	if err != nil {
		return errors.Wrap(err, "failed to call image_usbkey_dev")
	}
	if usbdev == "" {
		return errors.New("no USB key detected")
	}
	if err := h.CorruptUSBKey(ctx, usbdev); err != nil {
		return errors.Wrap(err, "failed to corrupt the USB")
	}
	usb.isCorrupted = true
	testing.ContextLog(ctx, "Enabling USB connection to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to enable USB connection to DUT")
	}
	testing.ContextLogf(ctx, "Sleeping %v to let USB become visible to dut", firmware.UsbVisibleTime)
	// GoBigSleepLint: It may take some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v s", firmware.UsbDisableTime)
	}
	return nil
}

func (usb *usbState) restoreUSB(ctx context.Context, h *firmware.Helper) error {
	if !usb.isCorrupted {
		return nil
	}
	testing.ContextLog(ctx, "Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to power off the USB")
	}
	testing.ContextLog(ctx, "Restoring USB")
	if err := h.RestoreUSBKey(ctx); err != nil {
		return errors.Wrap(err, "failed to restore the USB")
	}
	testing.ContextLog(ctx, "Enabling a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to enable the USB to DUT")
	}
	testing.ContextLogf(ctx, "Sleeping %v to let USB become visible to dut", firmware.UsbVisibleTime)
	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v s", firmware.UsbDisableTime)
	}
	return nil
}

func ctrlUBootFromUSB(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing Ctrl-U")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press Ctrl-U")
	}
	return nil
}

func menuUIBootFromUSB(ctx context.Context, h *firmware.Helper) error {
	menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create a new menu bypasser")
	}
	return menuBypasser.BypassDevBootUSB(ctx)
}

func selectBack(ctx context.Context, h *firmware.Helper) error {
	nvg, err := firmware.NewMenuNavigator(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create a new menu navigator")
	}
	testing.ContextLog(ctx, "Selecting \"Back\"")
	if err := nvg.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Back\"")
	}
	return nil
}

func ctrlDBootFromInternal(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing Ctrl-D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press Ctrl-D")
	}
	return nil
}

// restoreUSBOnFirmwareScreen restores USB and inserts usb to the DUT.
func (usb *usbState) restoreUSBOnFirmwareScreen(ctx context.Context, h *firmware.Helper) error {
	// Pressing space would lead DUT to the to-norm page, which would buy us
	// some time to restore usb, and bypass the fw screen timeout.
	if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
		testing.ContextLog(ctx, "Pressing space key to bypass fw screen timeout")
		if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press space")
		}
	}

	if err := usb.restoreUSB(ctx, h); err != nil {
		return errors.Wrap(err, "failed to restore USB")
	}

	if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
		// GoBigSleepLint: Simulate a specific speed of key presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v second", h.Config.KeypressDelay)
		}
		testing.ContextLog(ctx, "Pressing esc to return to the developer screen")
		if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press esc key")
		}
	}
	return nil
}
