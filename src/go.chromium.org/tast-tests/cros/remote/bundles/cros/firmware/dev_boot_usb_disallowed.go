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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootUSBDisallowed,
		Desc: "Verify that boot from the USB is not allowed when dev_boot_usb is disabled",
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
		Timeout:      2 * time.Hour,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DevBootUSBDisallowed(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// Set up USB when there is one present, and
	// for cases that depend on it.
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

	s.Log("Disabling dev_boot_usb")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0").Run(); err != nil {
		s.Fatal("Failed to set crossystem: ", err)
	}

	s.Log("Rebooting DUT to developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to cold reset dut: ", err)
	}
	s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Resetting firmware screen timeout")
	if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
		s.Fatal("Failed to press space key: ", err)
	}

	s.Log("Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		s.Logf("Failed to set pd data role to DFP: %.400s", err)
	}
	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set USBMux: ", err)
	}
	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}

	// On KeyboardDevSwitcher machines, pressing space triggers the
	// to_norm screen. Revert to the developer screen with the
	// esc key.
	if h.Config.ModeSwitcherType == firmware.KeyboardDevSwitcher {
		if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
			s.Fatal("Failed to press esc: ", err)
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %s (KeypressDelay): %v", h.Config.KeypressDelay, err)
		}
	}

	s.Log("Pressing Ctrl-U")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
		s.Fatal("Failed to press Ctrl-U: ", err)
	}
	// GoBigSleepLint: Simulate a specific speed of button presses.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		s.Fatalf("Failed to sleep for %v s, %v", h.Config.KeypressDelay, err)
	}
	// On MenuSwitcher machines, a message box would appear, suggesting that
	// external boot is disabled. Pressing the Enter key hits the back button,
	// and deactivates the box.
	if h.Config.ModeSwitcherType == firmware.MenuSwitcher {
		s.Log("Pressing Enter key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
			s.Fatal("Failed to press enter key: ", err)
		}
		// GoBigSleepLint: Simulate a specific speed of button presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %v s, %v", h.Config.KeypressDelay, err)
		}
	}
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		s.Fatal("Failed to press Ctrl-D: ", err)
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	testing.ContextLog(ctx, "Checking if DUT is in dev mode")
	isDevMode, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeDev)
	if err != nil {
		s.Fatal("Failed to get dut boot mode: ", err)
	}
	if !isDevMode {
		s.Fatal("Failed to boot from USB: ", err)
	}

	s.Log("Checking cbmem log for the displayed screens and usb boot disabled message")
	if err := verifyDisabledUSBBootFwLog(ctx, h, identifyDisabledUSBBootFwLog(h)); err != nil {
		saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
		if saveErr := h.SaveCBMEMLogs(ctx, saveLogPath); saveErr != nil {
			err = errors.Wrap(saveErr, err.Error())
		}
		s.Fatal("Failed to verify disabled usb boot from CBMEM: ", err)
	}
}

type disabledUSBBootFwLog struct {
	screenIds []fwCommon.FwScreenID
	logs      []string
}

func identifyDisabledUSBBootFwLog(h *firmware.Helper) disabledUSBBootFwLog {
	var data disabledUSBBootFwLog
	switch h.Config.ModeSwitcherType {
	case firmware.MenuSwitcher:
		data.screenIds = []fwCommon.FwScreenID{
			fwCommon.DeveloperMode,
		}
		data.logs = []string{`ui_display_screen: External boot is disabled`}
	case firmware.TabletDetachableSwitcher:
		data.screenIds = []fwCommon.FwScreenID{
			fwCommon.LegacyDeveloperWarningMenu,
			fwCommon.LegacyBlank,
			fwCommon.LegacyDeveloperWarningMenu,
			fwCommon.LegacyBlank,
		}
		data.logs = []string{`vb2_error_notify: USB booting is disabled`}
	case firmware.KeyboardDevSwitcher:
		data.screenIds = []fwCommon.FwScreenID{
			fwCommon.LegacyDeveloperWarning,
			fwCommon.LegacyDeveloperToNorm,
			fwCommon.LegacyDeveloperWarning,
			fwCommon.LegacyBlank,
		}
		data.logs = []string{`vb2_error_notify: VbBootDeveloper\(\) - USB booting is disabled`}
	}
	return data
}

func verifyDisabledUSBBootFwLog(ctx context.Context, h *firmware.Helper, expLogs disabledUSBBootFwLog) error {
	foundExpScreens, err := h.Reporter.CheckDisplayedScreens(ctx, expLogs.screenIds)
	if err != nil {
		return err
	}
	if !foundExpScreens {
		return errors.New("failed to find firmware screens as expected")
	}
	cbmemLog, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		return err
	}
	if err := h.ScanWithoutExpectedSequenceInSource(ctx, cbmemLog, expLogs.logs); err != nil {
		return err
	}
	return nil
}
