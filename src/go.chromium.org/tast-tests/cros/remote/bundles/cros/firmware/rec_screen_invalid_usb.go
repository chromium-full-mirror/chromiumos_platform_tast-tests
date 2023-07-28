// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecScreenInvalidUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify if DUT reaches RecoveryNoGood, or RecoveryInvalid screen invoked by invalid USB",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:    []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		Fixture: fixture.NormalMode,
		Timeout: 90 * time.Minute,
	})
}

func RecScreenInvalidUSB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	// Set up a valid usb for dut to recover later from NoGoodScreen.
	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}
	defer func() {
		// The dut might have booted from the usb.
		// Reboot the machine from main disk before
		// restoring the usb device.
		s.Log("Rebooting the DUT with cold reset")
		if err := resetDUT(ctx, h); err != nil {
			s.Fatal("Failed to cold reset the DUT: ", err)
		}
		if err := h.RestoreUSBKey(ctx); err != nil {
			s.Fatal("Failed to restore the USB: ", err)
		}
	}()
	if err := bootToNoGoodScreen(ctx, h); err != nil {
		s.Fatal("Failed to traverse NoGood screen: ", err)
	}
	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to power off the USB: ", err)
	}
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
		s.Fatal("Failed to boot from the USB: ", err)
	}
	screenNames := identifyFwScreens(h)
	verifyFwLog := firmware.ProcessLastBootFwLog{
		SaveLog:       true,
		SaveLogPath:   filepath.Join(s.OutDir(), "firmware.log"),
		VerifyScreens: screenNames,
	}
	if err := h.ScanLastBootFwLog(ctx, verifyFwLog); err != nil {
		s.Fatal("Failed to scan last boot fw log: ", err)
	}
}

func bootToNoGoodScreen(ctx context.Context, h *firmware.Helper) error {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		return err
	}
	testing.ContextLogf(ctx, "Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	if h.Config.ModeSwitcherType == firmware.MenuSwitcher {
		menuNavigator, err := firmware.NewMenuNavigator(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new menu navigator")
		}
		// Select 'Recovery using external storage' on the recovery screen.
		// Select 'Next' on the 'Get ready to recover your device' screen.
		// Select 'Next' on the 'Set up your external storage' screen.
		for press := 0; press < 3; press++ {
			if err := menuNavigator.SelectOption(ctx); err != nil {
				return err
			}
			testing.ContextLogf(ctx, "Sleeping for %s (KeypressDelay)", h.Config.KeypressDelay)
			// GoBigSleepLint: Simulate a specific speed of key press.
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrap(err, "failed to sleep")
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
	testing.ContextLog(ctx, "Sleeping for 2 secs to ensure rec invalid screen appears")
	// GoBigSleepLint: This sleep is necessary to accommodate for the delay
	// that the DUT takes in recognizing the USB as a valid/invalid device.
	// If the delay was too short, the rec invalid screen might not appear.
	// If the delay was too long, the firmware log might get over flooded.
	// When leasing a few duts and running this test remotely, we found a
	// duration of two seconds to be the most promising.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

func resetDUT(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to send cold reset command")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}
	return nil
}

func identifyFwScreens(h *firmware.Helper) []string {
	var expFwScreensInOrder []firmware.FwScreenID
	switch h.Config.ModeSwitcherType {
	case firmware.MenuSwitcher:
		expFwScreensInOrder = []firmware.FwScreenID{
			firmware.RecoverySelect,
			firmware.RecoveyDiskStep1,
			firmware.RecoveyDiskStep2,
			firmware.RecoveyDiskStep3,
			firmware.RecoveyInvalid,
			firmware.RecoverySelect,
		}
	default:
		expFwScreensInOrder = []firmware.FwScreenID{
			firmware.InsertScreen,
			firmware.RecoveyNoGood,
			firmware.InsertScreen,
		}
	}
	var expMatches []string
	for _, match := range expFwScreensInOrder {
		expMatches = append(expMatches, fmt.Sprintf(`(vb2ex_display_ui|vboot_draw_|ui_display).*screen=0x%x`, match))
	}
	return expMatches
}
