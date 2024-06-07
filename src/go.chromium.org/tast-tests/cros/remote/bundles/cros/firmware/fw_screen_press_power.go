// Copyright 2024 The ChromiumOS Authors
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
)

type fwScreenPressPwrParams struct {
	bootMode     fwCommon.BootMode
	bootToScreen fwCommon.FwScreenType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FwScreenPressPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify pressing the power button on a firmware screen shuts down the DUT",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2", "firmware_usb"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Timeout:      2 * time.Hour,
		Params: []testing.Param{{
			Name:    "dev_screen",
			Fixture: fixture.DevMode,
			Val: &fwScreenPressPwrParams{
				bootMode:     fwCommon.BootModeDev,
				bootToScreen: fwCommon.FwDeveloperScreen,
			},
		}, {
			Name:    "to_norm_screen",
			Fixture: fixture.DevMode,
			Val: &fwScreenPressPwrParams{
				bootMode:     fwCommon.BootModeDev,
				bootToScreen: fwCommon.FwToNormScreen,
			},
		}, {
			Name:    "broken_screen",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootMode:     fwCommon.BootModeNormal,
				bootToScreen: fwCommon.FwBrokenScreen,
			},
		}, {
			Name:    "rec_screen",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootMode:     fwCommon.BootModeNormal,
				bootToScreen: fwCommon.FwRecoveryScreen,
			},
		}, {
			Name:    "invalid_screen",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootMode:     fwCommon.BootModeNormal,
				bootToScreen: fwCommon.FwInvalidScreen,
			},
		}},
	})
}

func FwScreenPressPower(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	param := s.Param().(*fwScreenPressPwrParams)

	if param.bootToScreen == fwCommon.FwInvalidScreen {
		skipFlashUSB := false
		if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
			var err error
			skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
			if err != nil {
				s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
			}
		}
		cs := s.CloudStorage()
		if skipFlashUSB {
			cs = nil
		}
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBkey not working: ", err)
		}
		s.Log("Setting up an invalid USB")
		if err := setupInvalidUSB(ctx, h); err != nil {
			s.Fatal("Failed to set up invalid USB: ", err)
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure dut has booted: ", err)
		}
		if param.bootToScreen == fwCommon.FwInvalidScreen {
			if err := restoreUSB(ctx, h); err != nil {
				s.Fatal("Failed to restore USB: ", err)
			}
		}
	}(cleanupCtx)

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Log("Failed to create mode switcher: ", err)
	}
	if err := ms.RebootToFirmwareScreen(ctx, param.bootToScreen); err != nil {
		s.Fatal("Failed to reboot to firmware screen: ", err)
	}

	if err := fwScreenPressPowerOff(ctx, h, param.bootToScreen, ms); err != nil {
		s.Fatal("Failed to power off: ", err)
	}

	testing.ContextLog(ctx, "Pressing power key")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		s.Fatal("Failed to press power key: ", err)
	}

	if param.bootMode == fwCommon.BootModeDev {
		devModeBypasserParams := firmware.RunBypasser{
			BypasserMethod:        ms.BypassDevMode,
			RepeatBypasser:        true,
			WaitUntilDUTConnected: h.Config.DelayRebootToPing,
		}
		if err := ms.RunBypasserUntilDUTConnected(ctx, devModeBypasserParams); err != nil {
			s.Fatal("Failed to boot through dev mode: ", err)
		}
	} else {
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
	}

	if isExpMode, err := h.Reporter.CheckBootMode(ctx, param.bootMode); err != nil {
		s.Fatal("Failed to check boot mode: ", err)
	} else if !isExpMode {
		s.Fatal("Found unexpected boot mode")
	}
}

func fwScreenPressPowerOff(ctx context.Context, h *firmware.Helper, bootToScreen fwCommon.FwScreenType, ms *firmware.ModeSwitcher) error {
	if h.Config.IsDetachable && h.Config.ModeSwitcherType == firmware.MenuSwitcher {
		// Since power button has been overridden as a select button in the
		// fw screens for detachables, shutdown the DUT using the poweroff command.
		testing.ContextLog(ctx, "Setting power off")
		if err := ms.PowerOff(ctx); err != nil {
			return errors.Wrap(err, "failed to power off")
		}
	} else {
		// For TabletDetachableSwitcher, move to the "Poweroff" option and
		// press power key.
		if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher && bootToScreen == fwCommon.FwToNormScreen {
			if err := moveToPowerOffOnToNormScreen(ctx, h); err != nil {
				return errors.Wrap(err, "failed to move to power off on to_norm screen")
			}
		}
		testing.ContextLog(ctx, "Pressing power key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to press power key")
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
			return errors.Wrap(err, "failed to get G3 power state")
		}
	}
	return nil
}

func moveToPowerOffOnToNormScreen(ctx context.Context, h *firmware.Helper) error {
	nvg, err := firmware.NewMenuNavigator(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create menu navigator")
	}
	if err := firmware.MoveTo(ctx, h, nvg, 0, 2); err != nil {
		return errors.Wrap(err, "failed to move to menu")
	}
	return nil
}

func setupInvalidUSB(ctx context.Context, h *firmware.Helper) error {
	usbDev, err := h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
	if err != nil {
		return errors.Wrap(err, "failed to call image_usbkey_dev")
	}
	if usbDev == "" {
		return errors.New("no USB key detected")
	}
	if err := h.CorruptUSBKey(ctx, usbDev); err != nil {
		return errors.Wrap(err, "failed to corrupt the USB")
	}
	return nil
}

func restoreUSB(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to power off the USB")
	}
	testing.ContextLog(ctx, "Restoring the USB")
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
