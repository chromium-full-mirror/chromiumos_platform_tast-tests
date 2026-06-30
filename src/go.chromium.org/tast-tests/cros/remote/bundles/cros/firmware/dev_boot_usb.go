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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type devBootUSBTrigger int

const (
	triggerUSBBootWithKeyboard devBootUSBTrigger = iota
	triggerUSBBootWithMenu
	triggerUSBBootWithButton
	triggerUSBBootLateInsert
)

type devBootUSBParam struct {
	trigger devBootUSBTrigger
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootUSB,
		Desc: "Verify that boot-up from USB works from various triggers",
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
		Params: []testing.Param{{
			Name: "keyboard",
			Val: &devBootUSBParam{
				trigger: triggerUSBBootWithKeyboard,
			},
		}, {
			Name: "menu",
			Val: &devBootUSBParam{
				trigger: triggerUSBBootWithMenu,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.LegacyMenuUI, hwdep.MenuUI)),
		}, {
			Name: "button",
			Val: &devBootUSBParam{
				trigger: triggerUSBBootWithButton,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
		}, {
			Name: "late_insert",
			Val: &devBootUSBParam{
				trigger: triggerUSBBootLateInsert,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FirmwareUIType(hwdep.LegacyMenuUI, hwdep.LegacyClamshellUI)),
		},
		}})
}

func DevBootUSB(ctx context.Context, s *testing.State) {
	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	testOpt := s.Param().(*devBootUSBParam)

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

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

	s.Log("Enabling dev_boot_usb")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1").Run(); err != nil {
		s.Fatal("Failed to set crossystem dev_boot_usb to 1: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()
	// Set dev_boot_usb back to 0 at the end of the test.
	defer func(ctx context.Context) {
		if h.DUT.Connected(ctx) {
			if err := h.RebootWithSSHCommand(ctx, pv.BootMode); err != nil {
				s.Fatal("Failed to reconnect to dut: ", err)
			}
		} else {
			if err := h.EnsureDUTBooted(ctx); err != nil {
				s.Fatal("Failed to reconnect to dut: ", err)
			}
		}
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set crossystem dev_boot_usb to 0: ", err)
		}

		if state.RemoveServoChargerRequired && !state.IsServoChargerConnected {
			if err := h.SetDUTPower(ctx, true); err != nil {
				s.Fatal("Failed to connect charger: ", err)
			}
			state.IsServoChargerConnected = true
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
				s.Fatal("Failed to reconnect to the DUT: ", err)
			}
		}
	}(cleanupCtx)

	s.Log("Removing USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to remove the USB: ", err)
	}

	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			s.Error("Failed to capture EC UART: ", err)
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				s.Error("Failed to cancel capture EC UART: ", err)
			}
		}()
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
			s.Error("Failed to detect firmware screen: ", err)
		}
	}
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

	// Pressing Ctrl-U should invoke a beep sound, indicating that
	// there is no USB device.
	if testOpt.trigger == triggerUSBBootLateInsert {
		s.Log("Pressing Ctrl-U")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatal("Failed to press Ctrl-U: ", err)
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep for %s (KeypressDelay): %v", h.Config.KeypressDelay, err)
		}
		if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
			s.Fatal("Failed to press esc: ", err)
		}
	}

	if state.RemoveServoChargerRequired && state.IsServoChargerConnected {
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

	s.Log("Inserting a valid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to insert USB to DUT: ", err)
	}
	// GoBigSleepLint: It may take some time for usb mux state to
	// take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		s.Fatalf("Failed to sleep for %v s: %v", firmware.UsbDisableTime, err)
	}

	switch testOpt.trigger {
	case triggerUSBBootWithKeyboard, triggerUSBBootLateInsert:
		s.Log("Pressing Ctrl-U")
		if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
			s.Fatal("Failed to press Ctrl-U: ", err)
		}
	case triggerUSBBootWithMenu:
		menuBypasser, err := firmware.NewMenuBypasser(ctx, h)
		if err != nil {
			s.Fatal("Failed to build new menuBypasser: ", err)
		}
		if err := menuBypasser.BypassDevBootUSB(ctx); err != nil {
			s.Fatal(err, "Failed to bypass dev mode: ", err)
		}
	case triggerUSBBootWithButton:
		s.Log("Pressing and holding volume up button for 5 seconds")
		if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 5000); err != nil {
			s.Fatal("Failed to press volume up: ", err)
		}
	}
	s.Log("Waiting for DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if DUT is in usbdev mode")
	isUSBDev, err := h.Reporter.CheckBootMode(ctx, fwCommon.BootModeUSBDev)
	if err != nil {
		s.Fatal("Failed to get dut boot mode: ", err)
	}
	if !isUSBDev {
		s.Fatal("Failed to boot from the USB")
	}

	if testOpt.trigger == triggerUSBBootLateInsert {
		s.Log("Verifying that Ctrl-U was ineffective when there was no USB plugged")
		lateInsertMsg := map[firmware.ModeSwitcherType]string{
			firmware.TabletDetachableSwitcher: "No bootable kernel found on USB/SD",
			firmware.KeyboardDevSwitcher:      "no kernel found on USB",
		}
		output, err := h.Reporter.GetCBMEMLogs(ctx)
		if err != nil {
			s.Fatal("Failed to get CBMEM logs: ", err)
		}
		if err := h.ScanWithoutExpectedSequenceInSource(ctx, output, []string{lateInsertMsg[h.Config.ModeSwitcherType]}); err != nil {
			saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
			if saveErr := h.SaveCBMEMLogs(ctx, saveLogPath); saveErr != nil {
				err = errors.Wrap(saveErr, err.Error())
			}
			s.Fatalf("Failed to find %s from the CBMEM logs: %v", lateInsertMsg[h.Config.ModeSwitcherType], err)
		}
	}
}
