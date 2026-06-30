// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
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
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw", "firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.DevMode,
		Timeout:      2 * time.Hour,
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
	if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.Off); err != nil {
		s.Fatal("Failed to turn off usb keyboard: ", err)
	}
	if err := h.Servo.SetOnOff(ctx, servo.InitKeyboard, servo.On); err != nil {
		s.Fatal("Failed to turn on internal keyboard: ", err)
	}
	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Fatal("Failed to enable capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}()
	}

	s.Log("Removing USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove USB: ", err)
	}

	if err := h.DisableDevBootUSB(ctx); err != nil {
		s.Fatal("Failed to disable usb boot: ", err)
	}

	// For dm-default-key layouts, the dev image preservation requires an extra preservation step.
	// The binary will return success on all other layouts.
	if err := h.DUT.Conn().CommandContext(ctx, "/usr/local/bin/preserve_dev_image").Run(); err != nil {
		s.Fatal("Failed preserving dev image: ", err)
	}

	s.Log("Rebooting DUT to developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset dut: ", err)
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable after sending a warm reset: ", err)
	}

	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
			s.Log("Failed to detect firmware screen: ", err)
		}
	} else {
		s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
		// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
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
	defer func() {
		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			s.Log("Connecting servo charger")
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 90*time.Second)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
		}
	}()
	s.Log("Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		s.Logf("Failed to set pd data role to DFP: %.400s", err)
	}
	if err := h.ReturnToDeveloperScreen(ctx); err != nil {
		s.Fatal("Failed to return to developer screen: ", err)
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

	s.Log("Pressing Ctrl-U")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
		s.Fatal("Failed to press Ctrl-U: ", err)
	}
	// GoBigSleepLint: Simulate a specific speed of button presses.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		s.Fatalf("Failed to sleep for %v s, %v", h.Config.KeypressDelay, err)
	}
	s.Log("Removing USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove USB: ", err)
	}
	// On MenuSwitcher machines, a message box would appear, suggesting that
	// external boot is disabled. Pressing the Enter key hits the back button,
	// and deactivates the box.
	if h.Config.ModeSwitcherType == firmware.MenuSwitcher {
		s.Log("Pressing Enter key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
			s.Fatal("Failed to press enter key: ", err)
		}
		if h.HasAPFwState {
			if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
				s.Log("Failed to detect firmware screen: ", err)
			}
		} else {
			// GoBigSleepLint: Simulate a specific speed of button presses.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				s.Fatalf("Failed to sleep for %v s, %v", h.Config.KeypressDelay, err)
			}
		}
	}
	s.Log("Pressing Ctrl-D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		s.Fatal("Failed to press Ctrl-D: ", err)
	}

	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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
		s.Fatal("Failed to boot to dev mode")
	}

	s.Log("Checking cbmem log for the displayed screens and usb boot disabled message")
	if err := verifyDisabledUSBBootFwLog(ctx, h, getExpectedFwLogs(h)); err != nil {
		saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
		if saveErr := h.SaveCBMEMLogs(ctx, saveLogPath); saveErr != nil {
			err = errors.Wrap(saveErr, err.Error())
		}
		cbmemLog, getFwLogErr := h.Reporter.GetCBMEMLogs(ctx)
		if getFwLogErr != nil {
			s.Fatal("Failed to get cbmem logs: ", getFwLogErr)
		}
		drawingFailedStr := `Drawing failed`
		if strings.Contains(cbmemLog, drawingFailedStr) {
			s.Fatalf("Failed to verify disabled usb boot from CBMEM, got %v: %v", drawingFailedStr, err)
		}
		s.Fatal("Failed to verify disabled usb boot from CBMEM: ", err)
	}
}

func getExpectedFwLogs(h *firmware.Helper) []string {
	var logs []string
	switch h.Config.ModeSwitcherType {
	case firmware.MenuSwitcher:
		logs = []string{`(External boot is disabled|Dev mode external boot not allowed)`}
	case firmware.TabletDetachableSwitcher, firmware.KeyboardDevSwitcher:
		logs = []string{`USB booting is disabled`}
	}
	return logs
}

func verifyDisabledUSBBootFwLog(ctx context.Context, h *firmware.Helper, expLogs []string) error {
	cbmemLog, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		return err
	}
	if err := h.ScanWithoutExpectedSequenceInSource(ctx, cbmemLog, expLogs); err != nil {
		return err
	}
	return nil
}
