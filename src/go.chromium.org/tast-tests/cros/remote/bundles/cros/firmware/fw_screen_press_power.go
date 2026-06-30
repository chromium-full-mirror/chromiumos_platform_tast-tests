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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type fwScreenPressPwrParams struct {
	bootToScreen fwCommon.FwScreenType
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FwScreenPressPower,
		Desc: "Verify pressing the power button on a firmware screen shuts down the DUT",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Timeout:      2 * time.Hour,
		Params: []testing.Param{{
			Name:    "dev_screen",
			Fixture: fixture.DevMode,
			Val: &fwScreenPressPwrParams{
				bootToScreen: fwCommon.FwDeveloperScreen,
			},
		}, {
			Name:    "to_norm_screen",
			Fixture: fixture.DevMode,
			Val: &fwScreenPressPwrParams{
				bootToScreen: fwCommon.FwToNormScreen,
			},
		}, {
			Name:      "broken_screen",
			ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
			Fixture:   fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootToScreen: fwCommon.FwBrokenScreen,
			},
		}, {
			Name:    "rec_screen",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootToScreen: fwCommon.FwRecoveryScreen,
			},
			ExtraAttr: []string{"firmware_enabled", "firmware_meets_kpi"},
		}, {
			Name:    "invalid_screen",
			Fixture: fixture.NormalMode,
			Val: &fwScreenPressPwrParams{
				bootToScreen: fwCommon.FwInvalidScreen,
			},
			ExtraAttr:        []string{"firmware_enabled", "firmware_meets_kpi"},
			ExtraTestBedDeps: []string{tbdep.ServoUSBState("NORMAL")},
		}},
	})
}

func FwScreenPressPower(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
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
			if err := h.RestoreUSBKey(ctx); err != nil {
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

	s.Log("Sleeping for 2 seconds before pressing power key")
	// GoBigSleepLint: Wait a little while before waking up the DUT again.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	s.Log("Pressing power key")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
		s.Fatal("Failed to press power key: ", err)
	}

	if pv.BootMode == fwCommon.BootModeDev {
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
			currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
			if stateErr != nil {
				s.Fatalf("Failed to reconnect to DUT: %v, failed to check powerstate: %v", err, stateErr)
			}
			s.Fatalf("Failed to reconnect to the DUT and got %v power state: %v", currPowerState, err)
		}
	}

	if isExpMode, err := h.Reporter.CheckBootMode(ctx, pv.BootMode); err != nil {
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
		if bootToScreen == fwCommon.FwDeveloperScreen {
			if err := h.ByPassDevBootTimeout(ctx); err != nil {
				return errors.Wrap(err, "failed to bypass dev boot timeout")
			}
			if err := h.ReturnToDeveloperScreen(ctx); err != nil {
				return errors.Wrap(err, "failed to return to developer screen")
			}
		}
		// For TabletDetachableSwitcher, move to the "Poweroff" option and
		// press power key.
		if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher && bootToScreen == fwCommon.FwToNormScreen {
			if err := moveToPowerOffOnToNormScreen(ctx, h); err != nil {
				return errors.Wrap(err, "failed to move to power off on to_norm screen")
			}
		}
		testing.ContextLog(ctx, "Sleeping for 2 seconds before pressing power key")
		// GoBigSleepLint: It may take some time for the DUT to be ready to
		// accept power key press.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		testing.ContextLog(ctx, "Pressing power key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return errors.Wrap(err, "failed to press power key")
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
			return errors.Wrap(err, "failed to get G3 power state after pressing the power key on the firmware screen")
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
