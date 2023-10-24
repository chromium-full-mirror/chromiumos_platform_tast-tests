// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type caseName int

const (
	insertToOption caseName = iota
	ctrlD
	keyboardUpDown
	keyboardF9F10
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DetachableUntrustedKeyboard,
		Desc: "Verify untrusted keys from an attached keyboard have no effect on the firmware screen",
		Contacts: []string{
			"cienet-firmware@cienet.corp-partner.google.com",
			"chromeos-firmware@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_detachable"},
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.NormalMode,
		// To-do: Find a way to preserve firmware logs on soraka and nocturne.
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable), hwdep.SkipOnModel("soraka", "nocturne")),
		Timeout:      20 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DetachableUntrustedKeyboard(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Requiring config: ", err)
	}

	// Upload firmware log to Testhaus at the end of the test.
	defer func() {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT booted: ", err)
		}
		output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		destPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := ioutil.WriteFile(destPath, []byte(output), 0666); err != nil {
			s.Fatal("Failed to write firmware log: ", err)
		}
	}()

	for _, tc := range []struct {
		casename       caseName
		keysToBeTested []string
	}{
		// Expect that DUT moves form insert screen to to_dev screen.
		{insertToOption, []string{""}},
		// Expect that after pressing ctrlD three times, the DUT stays at the insert screen.
		{ctrlD, []string{"ctrlD", "ctrlD", "ctrlD"}},
		// Expect to_dev screen menu selection at default.
		{keyboardUpDown, []string{"volumeUpDown", "keyboardUp", "keyboardDown", "enter"}},
		// Expect to_dev screen menu selection at default.
		{keyboardF9F10, []string{"volumeUpDown", "keyBoardF9", "keyboardF10", "enter"}},
	} {
		if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to enable recovery mode: ", err)
		}

		// GoBigSleepLint: Sleep for model specific time to wait for firmware screen.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s", h.Config.FirmwareScreen)
		}

		for _, key := range tc.keysToBeTested {
			s.Logf("Pressing %s", key)
			if err := pressKeyOnFWScreen(ctx, h, key); err != nil {
				s.Fatal("Failed to press: ", err)
			}
		}
		// Run bootupDUT prior to scanning firmware log.
		if err := bootupDUT(ctx, h); err != nil {
			s.Fatal("While attempting to boot: ", err)
		}

		if err := func() error {
			s.Log("Waiting for DUT to boot")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}

		s.Log("Verifying firmware log")
		if err := checkScreenFromFirmwareLog(ctx, h, h.Board, tc.casename); err != nil {
			s.Fatal("Failed to check screen from firmware log: ", err)
		}
		// Disable dev request here so that a cold reset would reboot DUT directly back to normal mode.
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "disable_dev_request=1").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set disable_dev_request: ", err)
		}
	}
}

// bootupDUT presses volumeUpDown, volumeUp, and enter to boot Kukui
// devices to developer mode. On Strongbad machines, bootupDUT presses
// volumeUpDown, and performs ec command 'apreset'. The purpose of bootupDUT
// is to boot the dut to ChromeOS with firmware log preserved, prior to scan.
func bootupDUT(ctx context.Context, h *firmware.Helper) error {
	switch h.Board {
	case "kukui":
		keysToBootIntoDev := []string{"volumeUpDown", "volumeUp", "enter"}
		for _, key := range keysToBootIntoDev {
			if err := pressKeyOnFWScreen(ctx, h, key); err != nil {
				return errors.Wrap(err, "failed to press insertToDev buttons")
			}
		}
	case "strongbad":
		if err := pressKeyOnFWScreen(ctx, h, "volumeUpDown"); err != nil {
			return errors.Wrap(err, "failed to press insertToDev buttons")
		}
		if err := h.Servo.RunECCommand(ctx, "apreset"); err != nil {
			return errors.Wrap(err, "failed to set apreset")
		}
	default:
		return errors.Errorf("unable to identify board %s", h.Board)
	}
	return nil
}

// checkScreenFromFirmwareLog checks whether the insert screen
// and to-dev screen were triggered, and recorded by the firmware log.
func checkScreenFromFirmwareLog(ctx context.Context, h *firmware.Helper, board string, casename caseName) error {
	output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
	if err != nil {
		return errors.Wrap(err, "failed to read firmware log")
	}

	// fwLogContains lists the expected messages to find in firmware log, as a result
	// of various presses on firmware screens.
	fwLogContains := map[string]map[caseName][]string{
		"kukui": {
			// Expect both insert screen and to-dev screen to be recorded.
			// Also, at the to-dev screen, the menu selection should stay at
			// the default position of "cancel", with selected_index=1.
			// Expected no change on the value of selected_index.
			insertToOption: {
				`vboot_draw_ui: screen=0x202.*selected_index=0`,
				`vboot_draw_ui: screen=0x20d.*selected_index=1`,
			},
			// Pressing ctrlD three times at the insert screen should have no effect.
			// Verify this by scanning for three consecutive presses, and no additional
			// screens in between.
			ctrlD: {
				`vb2_handle_menu_input: pressed key 0x4\nvb2_handle_menu_input: pressed key 0x4\nvb2_handle_menu_input: pressed key 0x4`,
			},
			// Pressing keyboard up or down triggers a beep sound, which is recorded
			// in the firmware log as untrusted inputs.
			keyboardUpDown: {
				`vb2_error_notify: vb2_handle_menu_input\(\) - Untrusted \(USB keyboard\) input disabled`,
			},
			// Pressing keyboard F9 and F10 should have no effect on the menu.
			// Expect the same information to be found as the insertToOption case.
			keyboardF9F10: {
				`vboot_draw_ui: screen=0x202.*selected_index=0`,
				`vboot_draw_ui: screen=0x20d.*selected_index=1`,
			},
		},
		"strongbad": {
			// Expect both insert screen and to-dev screen to be recorded,
			// and menu selections at default.
			insertToOption: {
				`vb2ex_display_ui: screen=0x200.*selected_item=2`,
				`vb2ex_display_ui: screen=0x202.*selected_item=1`,
			},
			// Pressing ctrlD once at the insert screen leads to the to-dev screen.
			// Additional ctrlD presses at the to-dev screen should have no effects.
			// Expect both insert screen and to-dev screen to be recorded,
			// and menu selections at default.
			ctrlD: {

				`vb2ex_display_ui: screen=0x200.*selected_item=2`,
				`vb2ex_display_ui: screen=0x202.*selected_item=1`,
			},
			// Pressing keyboard up down, and then enter, triggers a beep sound, which is recorded
			// in the firmware log as untrusted inputs.
			keyboardUpDown: {
				`recovery_to_dev_confirm_action: Reject untrusted ENTER confirmation`,
				`ui_display_screen: Use built-in keyboard to confirm`,
			},
			// Pressing keyboard F9 and F10 should have no effect on the menu.
			// Expect the same information to be found as the insertToOption case.
			keyboardF9F10: {
				`vb2ex_display_ui: screen=0x200.*selected_item=2`,
				`vb2ex_display_ui: screen=0x202.*selected_item=1`,
			},
		},
	}
	// fwLogExcludes lists the messages to exclude from firmware log, as a way to confirm
	// that certain keys do not work on firmware screens. Note that cases, such as insertToOption,
	// keyboardUpDown, and ctrlD have empty lists, because their counterparts in fwLogContains
	// already suffice.
	fwLogExcludes := map[string]map[caseName][]string{
		`kukui`: {
			insertToOption: nil,
			keyboardUpDown: nil,
			ctrlD:          nil,
			keyboardF9F10: {
				`vboot_draw_ui: screen=0x20d.*selected_index=0(.|\n)*vboot_draw_ui: screen=0x202`,
				`vboot_draw_ui: screen=0x20d.*selected_index=2(.|\n)*vboot_draw_ui: screen=0x202`,
			},
		},
		"strongbad": {
			insertToOption: nil,
			keyboardUpDown: nil,
			ctrlD:          nil,
			keyboardF9F10: {
				`vb2ex_display_ui: screen=0x202.*selected_item=0`,
				`vb2ex_display_ui: screen=0x202.*selected_item=2`,
			},
		},
	}

	if _, ok := fwLogContains[board][casename]; !ok {
		return errors.Errorf("Unknwon fw-log-contains for board %v", board)
	}
	for _, reContain := range fwLogContains[board][casename] {
		testing.ContextLogf(ctx, "verifing %s", reContain)
		re := regexp.MustCompile(reContain)
		if match := re.FindStringSubmatch(string(output)); match == nil {
			return errors.Errorf("failed to verify log containing: %s", reContain)
		}
	}

	if _, ok := fwLogExcludes[board][casename]; !ok {
		return errors.Errorf("Unknwon fw-log-excludes for board %v", board)
	}
	for _, reExclude := range fwLogExcludes[board][casename] {
		testing.ContextLogf(ctx, "verifing %s doesn't exist", reExclude)
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
		err = h.Servo.PressUSBKey(ctx, "<enter>", servo.DurTab)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to press %s", key)
	}
	// GoBigSleepLint: Sleep for model specific time to ensure keypresses effective.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrap(err, "failed to sleep for 1s")
	}
	return nil
}
