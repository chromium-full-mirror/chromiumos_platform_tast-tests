// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DetachableUntrustedKeyboard,
		Desc: "Verify untrusted keys from an attached keyboard have no effect on the firmware screen",
		Contacts: []string{
			"cienet-firmware@cienet.corp-partner.google.com",
			"chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable)),
		Timeout:      20 * time.Minute,
	})
}

func DetachableUntrustedKeyboard(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to init config: ", err)
	}

	for _, tc := range []struct {
		keysToBeTested []string
		recoverDUTConn func(context.Context, *firmware.Helper) error
		checkScreen    string
	}{
		// Expect that DUT moves form insert screen to to_dev screen.
		{nil, insertToDev, "insertAndToDevScreen"},
		// Expect that after pressing ctrlD three times, the DUT stays at the insert screen.
		{[]string{"ctrlD", "ctrlD", "ctrlD"}, insertToDev, "ctrlD"},
		// Expect to_dev screen menu selection at default.
		{[]string{"volumeUpDown", "keyboardUp", "keyboardDown", "enter"}, insertToDev, "insertAndToDevScreen"},
		// Expect to_dev screen menu selection at default.
		{[]string{"volumeUpDown", "keyBoardF9", "keyboardF10", "enter"}, insertToDev, "insertAndToDevScreen"},
	} {
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to power off usbkey: ", err)
		}

		// Power cycle the DUT to clear the firmware log, so that records prior
		// to this point are wiped.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Failed to power off DUT: ", err)
		}
		// Soraka would stay at S5 after setting its power state off.
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
			s.Fatal("Failed to get power state at G3 or S5: ", err)
		}

		s.Log("Booting the DUT to recovery mode")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}
		s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
		}

		for _, key := range tc.keysToBeTested {
			s.Logf("Pressing %s", key)
			if err := pressKeyOnFWScreen(ctx, h, key); err != nil {
				s.Fatal("Failed to press: ", err)
			}
			// Short delay between each presses.
			if err := testing.Sleep(ctx, 100*time.Millisecond); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}
		// Power cycles would clear records saved until now in firmware log.
		// But, allowing the DUT to boot directly from recovery to dev did not.
		if err := tc.recoverDUTConn(ctx, h); err != nil {
			s.Fatal("While attempting to boot: ", err)
		}

		if err := func() error {
			s.Log("Waiting for DUT to boot")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}

		s.Log("Verifying firmware log")
		if err := checkScreenFromFirmwareLog(ctx, h, tc.checkScreen); err != nil {
			s.Fatal("Failed to check screen from firmware log: ", err)
		}
		// Disable dev request here so that a cold reset would reboot DUT directly back to normal mode.
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "disable_dev_request=1").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set disable_dev_request: ", err)
		}

		// Run a cold reset to reboot dut back to normal mode.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to reset DUT to normal mode: ", err)
		}
		if err := func() error {
			s.Log("Waiting for reconnection to DUT")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}
}

// insertToDev presses volumeUpDown, volumeUp, and enter to boot DUT to developer mode.
func insertToDev(ctx context.Context, h *firmware.Helper) error {
	keysToBootIntoDev := []string{"volumeUpDown", "volumeUp", "enter"}
	for _, key := range keysToBootIntoDev {
		if err := pressKeyOnFWScreen(ctx, h, key); err != nil {
			return errors.Wrap(err, "failed to press insertToDev buttons")
		}
	}
	return nil
}

// checkScreenFromFirmwareLog checks whether the insert screen
// and to-dev screen were triggered, and recorded by the firmware log.
func checkScreenFromFirmwareLog(ctx context.Context, h *firmware.Helper, option string) error {
	output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
	if err != nil {
		return errors.Wrap(err, "failed to read firmware log")
	}

	var reContains, reExcludes []string
	switch option {
	case "ctrlD":
		// Pressing ctrlD three times at the insert screen should have no effect.
		// Verify this by scanning for three consecutive presses, and no additional
		// screens in between.
		reContains = append(reContains,
			`vboot_draw_ui: screen=0x202(.|\n)*vb2_handle_menu_input: pressed key 0x4\nvb2_handle_menu_input: pressed key 0x4\nvb2_handle_menu_input: pressed key 0x4\n`)
	case "insertAndToDevScreen":
		// Expect both insert screen and to-dev screen to be recorded.
		// Also, at the to-dev screen, the menu selection should stay at
		// the default position of "cancel", with selected_index=1.
		// Expected no change on the value of selected_index.
		reContains = append(reContains, `vboot_draw_ui: screen=0x202(.|\n)*vboot_draw_ui: screen=0x20d`)
		reExcludes = append(reExcludes, `vboot_draw_ui: screen=0x20d(.|\n)*selected_index=0(.|\n)*vboot_draw_ui: screen=0x202`)
		reExcludes = append(reExcludes, `vboot_draw_ui: screen=0x20d(.|\n)*selected_index=2(.|\n)*vboot_draw_ui: screen=0x202`)
	default:
		return errors.Errorf("Unable to identify screen: %s", option)
	}

	for _, reContain := range reContains {
		re := regexp.MustCompile(reContain)
		if match := re.FindStringSubmatch(string(output)); match == nil {
			return errors.Errorf("failed to verify log containing: %s", reContain)
		}
	}
	for _, reExclude := range reExcludes {
		re := regexp.MustCompile(reExclude)
		if match := re.FindStringSubmatch(string(output)); match != nil {
			return errors.Errorf("found %s unexpectedly in firmware log", reExclude)
		}
	}
	return nil
}

func pressKeyOnFWScreen(ctx context.Context, h *firmware.Helper, key string) error {
	var err error
	switch key {
	case "ctrlD":
		err = h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab)
	case "volumeUpDown":
		err = h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 100)
	case "volumeUp":
		err = h.Servo.SetInt(ctx, servo.VolumeUpHold, 100)
	case "keyboardUp":
		err = h.Servo.PressUSBKey(ctx, "<uparrow>", servo.DurTab)
	case "keyboardDown":
		err = h.Servo.PressUSBKey(ctx, "<downarrow>", servo.DurTab)
	case "keyBoardF9":
		err = h.Servo.PressUSBKey(ctx, "<f9>", servo.DurTab)
	case "keyboardF10":
		err = h.Servo.PressUSBKey(ctx, "<f10>", servo.DurTab)
	case "enter":
		err = h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to press %s", key)
	}
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for 1s")
	}
	return nil
}
