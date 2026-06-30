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
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecScreenInvalidUSB,
		Desc: "Verify if DUT reaches RecoveryNoGood, or RecoveryInvalid screen invoked by invalid USB",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  append([]string{tbdep.ServoUSBState("NORMAL")}, tbdep.ServoPresentAndWorking...),
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		Vars:         []string{"firmware.skipFlashUSB"},
		Fixture:      fixture.NormalMode,
		Timeout:      120 * time.Minute,
	})
}

func RecScreenInvalidUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	type bootUSBTimeout struct {
		err error
	}
	bootUSBTimeoutErr := bootUSBTimeout{}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	// Set up a valid usb for dut to recover later from NoGoodScreen.
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
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	var state firmware.CheckAndSetServoCharger = h.CheckServoChargerBeforeBootingFromUSB(ctx)

	defer func() {
		// The dut might have booted from the usb.
		// Reboot the machine from main disk before
		// restoring the usb device.
		if err := resetDUT(ctx, h, &state); err != nil {
			s.Fatal("Failed to cold reset the DUT: ", err)
		}
		if bootUSBTimeoutErr.err != nil {
			s.Log("Verifying the expected recovery reasons from event log")
			newEvents, err := h.Reporter.EventlogList(ctx)
			if err != nil {
				s.Error("Failed to find events: ", err)
			}
			checkRecoveryReasons := []reporters.RecoveryReason{reporters.RecoveryReasonROManual, reporters.RecoveryReasonLegacy, reporters.RecoveryReasonNotRequested}
			if err := h.Reporter.CheckRecoveryEventsInEventLog(ctx, newEvents, checkRecoveryReasons); err != nil {
				s.Error("Failed to check event logs for recovery reasons: ", err)
			} else {
				s.Error("Found DUT booted to the broken screen")
			}
			saveEventLogPath := filepath.Join(s.OutDir(), "eventlog.txt")
			if err := h.SaveEventLog(ctx, saveEventLogPath); err != nil {
				s.Error("Failed to save event log: ", err)
			}
		}
		if err := h.RestoreUSBKey(ctx); err != nil {
			s.Error("Failed to restore the USB: ", err)
		}
		if !state.IsServoChargerConnected && state.RemoveServoChargerRequired {
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

	if err := bootToNoGoodScreen(ctx, h, &state); err != nil {
		s.Fatal("Failed to traverse NoGood screen: ", err)
	}
	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to power off the USB: ", err)
	}
	if !h.HasAPFwState {
		s.Log("Restoring the USB")
		if err := h.RestoreUSBKey(ctx); err != nil {
			s.Fatal("Failed to restore the USB: ", err)
		}
		s.Log("Enabling a valid USB to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to enable the USB to DUT: ", err)
		}
		s.Log("Checking if DUT boots from the USB")
		if err := h.WaitDUTConnectDuringBootFromUSB(ctx, true); err != nil {
			if errors.As(err, &context.DeadlineExceeded) {
				bootUSBTimeoutErr.err = err
			}
			s.Fatal("Failed to boot from USB: ", err)
		}

		match, err := h.Reporter.CheckDisplayedScreens(ctx, identifyFwScreens(h))
		if err != nil {
			s.Fatal("Failed to verify firmware screen data: ", err)
		}
		if !match {
			saveLogPath := filepath.Join(s.OutDir(), "firmware.log")
			if err := h.SaveCBMEMLogs(ctx, saveLogPath); err != nil {
				s.Fatal("Failed to save firmware log while matching firmware screen data: ", err)
			}
			s.Fatal("Failed to find matching firmware screen data")
		}
	}
}

func bootToNoGoodScreen(ctx context.Context, h *firmware.Helper, state *firmware.CheckAndSetServoCharger) (retErr error) {
	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			return errors.Wrap(err, "failed to enable capture EC UART")
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "failed to cancel capture EC UART"))
			}
		}()
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		return err
	}

	if state.IsServoChargerConnected && state.RemoveServoChargerRequired {
		if err := h.SetDUTPower(ctx, false); err != nil {
			return errors.Wrap(err, "failed to remove charger")
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// pressing keys.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	testing.ContextLog(ctx, "Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	if h.HasAPFwState && (h.Board == "dedede" || h.Board == "corsola") {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			return errors.Wrap(err, "failed to enable capture EC UART")
		}
		defer func() {
			if err := closeUART(ctx); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "failed to cancel capture EC UART"))
			}
		}()
	}
	if h.Config.ModeSwitcherType == firmware.MenuSwitcher {
		menuNavigator, err := firmware.NewMenuNavigator(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new menu navigator")
		}
		// Select 'Recovery using external storage' on the recovery screen.
		// Select 'Next' on the 'Get ready to recover your device' screen.
		// Select 'Next' on the 'Set up your external storage' screen.
		for _, id := range []fwCommon.FwScreenID{
			fwCommon.RecoveryDiskStep1,
			fwCommon.RecoveryDiskStep2,
			fwCommon.RecoveryDiskStep3,
		} {
			if err := menuNavigator.SelectOption(ctx); err != nil {
				return err
			}
			if h.HasAPFwState {
				if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, id); err != nil {
					return errors.Wrap(err, "failed to detect firmware screen")
				}
			} else {
				testing.ContextLogf(ctx, "Sleeping for %s (KeypressDelay)", h.Config.KeypressDelay)
				// GoBigSleepLint: Simulate a specific speed of key press.
				if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
					return errors.Wrap(err, "failed to sleep")
				}
			}
		}
	}
	usbdev, err := h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
	if err != nil {
		return errors.Wrap(err, "failed to call image_usbkey_dev")
	}
	if usbdev == "" {
		return errors.New("no USB key detected")
	}
	testing.ContextLogf(ctx, "Sleeping %s to let USB become visible to servo host", firmware.UsbVisibleTime)
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, firmware.UsbVisibleTime); err != nil {
		return errors.Wrapf(err, "failed to sleep for %s", firmware.UsbVisibleTime)
	}
	// An invalid USB is required to check for the NOGOOD screen.
	if err := h.CorruptUSBKey(ctx, usbdev); err != nil {
		return errors.Wrap(err, "failed to corrupt the USB")
	}
	testing.ContextLog(ctx, "Enabling an invalid USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to enable the USB to DUT")
	}
	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.RecoveryInvalid); err != nil {
			return errors.Wrap(err, "failed to detect firmware screen")
		}
	} else {
		testing.ContextLogf(ctx, "Sleeping for %s to ensure rec invalid screen appears", h.Config.RecInvalidScreen)
		// GoBigSleepLint: This sleep is necessary to accommodate for the delay
		// that the DUT takes in recognizing the USB as a valid/invalid device.
		// If the delay was too short, the rec invalid screen might not appear.
		// If the delay was too long, the firmware log might get over flooded.
		// When leasing a few duts and running this test remotely, we found a
		// duration of two seconds to be the most promising for most of the boards,
		// and five seconds for boards, such as sarien, coral, octopus, eve, nami
		// and grunt.
		if err := testing.Sleep(ctx, h.Config.RecInvalidScreen); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	return nil
}

func resetDUT(ctx context.Context, h *firmware.Helper, state *firmware.CheckAndSetServoCharger) error {
	if h.DUT.Connected(ctx) && !state.IsServoChargerConnected && state.RemoveServoChargerRequired {
		// Applying cold reset with the function h.Servo.SetPowerState could lead to the
		// 'EC: No data was sent from the pty' error. Call a reboot command instead.
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
			return errors.Wrap(err, "failed to run reboot command")
		}
		waitUnreachableCtx, cancelWaitUnreachable := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitUnreachable()
		if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
			return errors.Wrap(err, "failed to wait for DUT to be unreachable after reboot")
		}
	} else {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			return errors.Wrap(err, "failed to reboot the DUT")
		}
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}

func identifyFwScreens(h *firmware.Helper) []fwCommon.FwScreenID {
	var expFWScreensInOrder []fwCommon.FwScreenID
	switch h.Config.ModeSwitcherType {
	case firmware.MenuSwitcher:
		expFWScreensInOrder = []fwCommon.FwScreenID{
			fwCommon.RecoverySelect,
			fwCommon.RecoveryDiskStep1,
			fwCommon.RecoveryDiskStep2,
			fwCommon.RecoveryDiskStep3,
			fwCommon.RecoveryInvalid,
			fwCommon.RecoverySelect,
		}
	default:
		expFWScreensInOrder = []fwCommon.FwScreenID{
			fwCommon.LegacyRecoveryInsert,
			fwCommon.LegacyRecoveryNoGood,
			fwCommon.LegacyRecoveryInsert,
			fwCommon.LegacyBlank,
		}
	}
	return expFWScreensInOrder
}
