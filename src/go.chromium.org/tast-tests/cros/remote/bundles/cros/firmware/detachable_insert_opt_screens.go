// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testKeys int

const (
	powerBtn testKeys = iota
	powerBtnUntilPoweroff
	volumeUpDown
	volumeUp
	volumeDown
	recBtn
)

type testCase int

const (
	powerBtnOnInsertMenu testCase = iota
	volumeUpDownEffective
	volumeUpDownUndetected
)

type fwLogInfo struct {
	fwScreenID  firmware.FwScreenID
	selectedIdx int
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DetachableInsertOptScreens,
		Desc: "Confirm insert and option screen behaviors on detachables",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_detachable"},
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.NormalMode,
		// Preserving the firmware log on Soraka and Nocturne requires modifying Kconfig
		// under coreboot. Skip because they will soon become EOF devices.
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable), hwdep.SkipOnModel("soraka", "nocturne")),
		Timeout:      30 * time.Minute,
	})
}

func DetachableInsertOptScreens(ctx context.Context, s *testing.State) {
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

	for _, tc := range []struct {
		caseName          testCase
		buttonPresses     []testKeys
		steps             map[string][]func(ctx context.Context, h *firmware.Helper) error
		fwLogDataSequence map[string][]fwLogInfo
	}{
		{
			caseName:      powerBtnOnInsertMenu,
			buttonPresses: []testKeys{powerBtnUntilPoweroff},
			steps: map[string][]func(ctx context.Context, h *firmware.Helper) error{
				"kukui":     {waitForPowerOff, setDUTPowerOn},
				"strongbad": {waitForPowerOff, setDUTPowerOn},
			},
			fwLogDataSequence: nil,
		},
		{
			/*
				Below shows the recorded log when volume up and down buttons are effective:
				recovery_ui: waiting for a recovery image
				vb2_log_menu_change: =============== Recovery INSERT Screen ===============
				vboot_draw_ui: screen=0x202 locale=0, selected_index=0,disabled_idx_mask=0x0
				load_archive: loading locale_en.bin
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Cancel ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=1,disabled_idx_mask=0x0
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Power Off ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=2,disabled_idx_mask=0x0
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Cancel ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=1,disabled_idx_mask=0x0
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Confirm Disabling OS Verification ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=0,disabled_idx_mask=0x0
				to_dev_action: Enabling dev-mode...
			*/
			caseName:      volumeUpDownEffective,
			buttonPresses: []testKeys{volumeUpDown, volumeDown, volumeUp},
			steps: map[string][]func(ctx context.Context, h *firmware.Helper) error{
				"kukui":     {enableDevMode},
				"strongbad": {apResetUsingECCmd},
			},
			fwLogDataSequence: map[string][]fwLogInfo{
				"kukui": {
					{firmware.InsertScreen, 0},
					{firmware.OptionScreen, 1},
					{firmware.OptionScreen, 2},
					{firmware.OptionScreen, 1},
					{firmware.OptionScreen, 0},
				},
				"strongbad": {
					{firmware.InsertScreenMenuSwitcher, 2},
					{firmware.OptionScreenMenuSwitcher, 1},
					{firmware.OptionScreenMenuSwitcher, 2},
					{firmware.OptionScreenMenuSwitcher, 1},
				},
			},
		},
		{
			/*
				Below shows the recorded log if the volume up and down buttons weren't picked up:
				recovery_ui: waiting for a recovery image
				vb2_log_menu_change: =============== Recovery INSERT Screen ===============
				vboot_draw_ui: screen=0x202 locale=0, selected_index=0,disabled_idx_mask=0x0
				load_archive: loading locale_en.bin
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Cancel ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=1,disabled_idx_mask=0x0
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Cancel ]
				vb2_log_menu_change: ================ TO_DEV Confirmation Menu ================ [ Confirm Disabling OS Verification ]
				vboot_draw_ui: screen=0x20d locale=0, selected_index=0,disabled_idx_mask=0x0
				to_dev_action: Enabling dev-mode...
				SetVirtualDevMode: Enabling developer mode...
			*/
			caseName:      volumeUpDownUndetected,
			buttonPresses: []testKeys{volumeUpDown, volumeUpDown},
			steps: map[string][]func(ctx context.Context, h *firmware.Helper) error{
				"kukui":     {enableDevMode},
				"strongbad": {apResetUsingECCmd},
			},
			fwLogDataSequence: map[string][]fwLogInfo{
				"kukui": {
					{firmware.InsertScreen, 0},
					{firmware.OptionScreen, 1},
					{firmware.OptionScreen, 0},
				},
				"strongbad": {
					{firmware.InsertScreenMenuSwitcher, 2},
					{firmware.OptionScreenMenuSwitcher, 1},
				},
			},
		},
	} {

		if err := ms.EnableRecMode(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to enable recovery mode: ", err)
		}

		for _, key := range tc.buttonPresses {
			if err := pressBtnOnFWScreen(ctx, h, key); err != nil {
				s.Fatal("Failed to press: ", err)
			}
		}

		for _, fnc := range tc.steps[h.Board] {
			if err := fnc(ctx, h); err != nil {
				s.Fatal("Unexpected error: ", err)
			}
		}

		if tc.fwLogDataSequence != nil {
			if err := verifyFirmwareLog(ctx, h, tc.fwLogDataSequence[h.Board], tc.caseName); err != nil {
				s.Fatal("Unexpected error in verifying firmware log: ", err)
			}
		}

		// Disable dev request here so the next reboot would bring the DUT back to normal mode.
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "disable_dev_request=1").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to set disable_dev_request: ", err)
		}

	}
}

// pressBtnOnFWScreen presses a variety of buttons when requested.
func pressBtnOnFWScreen(ctx context.Context, h *firmware.Helper, key testKeys) error {
	var err error
	switch key {
	case volumeUpDown:
		err = h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 100)
	case volumeUp:
		err = h.Servo.SetInt(ctx, servo.VolumeUpHold, 100)
	case volumeDown:
		err = h.Servo.SetInt(ctx, servo.VolumeDownHold, 100)
	case powerBtn:
		err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab)
	case powerBtnUntilPoweroff:
		err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonNoPowerdShutdown))
	default:
		err = errors.New("no key detected")
	}
	if err != nil {
		return errors.Wrapf(err, "failed to press %v", key)
	}
	// GoBigSleepLint: Simulate a specific speed of button presses.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrap(err, "failed to sleep between button presses")
	}
	return nil
}

// verifyFirmwareLog checks whether the insert screen
// and option screen were triggered, and recorded by the firmware log.
func verifyFirmwareLog(ctx context.Context, h *firmware.Helper, data []fwLogInfo, caseName testCase) error {
	var expMatches []string
	for _, args := range data {
		match := fmt.Sprintf(`(vboot_draw_ui|vb2ex_display_ui): screen=0x%x.*locale=0, selected_(index|item)=%d`, args.fwScreenID, args.selectedIdx)
		expMatches = append(expMatches, match)
	}
	output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
	if err != nil {
		return errors.Wrap(err, "failed to read firmware log")
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		if len(expMatches) > 0 {
			if match := regexp.MustCompile(expMatches[0]).FindStringSubmatch(scanner.Text()); match != nil {
				testing.ContextLogf(ctx, "Found screen %x with selected_index %d", data[0].fwScreenID, data[0].selectedIdx)
				expMatches, data = expMatches[1:], data[1:]
			}
			if caseName == volumeUpDownUndetected {
				menuChange := `(vboot_draw_ui|vb2ex_display_ui): screen=0x(202|20d).*locale=0, selected_(index|item)=2`
				if match := regexp.MustCompile(menuChange).FindStringSubmatch(scanner.Text()); match != nil {
					return errors.New("Unexpectedly found power off option selected on the menu")
				}
			}
		}
	}
	if len(expMatches) != 0 {
		return errors.Errorf("unable to find screen %x with selected_index %d", data[0].fwScreenID, data[0].selectedIdx)
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to scan firmware log")
	}
	return nil
}

// waitForPowerOff waits for the DUT to reach the G3 power state.
func waitForPowerOff(ctx context.Context, h *firmware.Helper) error {
	return h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3")
}

// setDUTPowerOn sends servo command to power on the DUT.
func setDUTPowerOn(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
		return errors.Wrap(err, "failed to set DUT's power on")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

// enableDevMode selects the to-dev option while at the to-dev confirmation menu.
func enableDevMode(ctx context.Context, h *firmware.Helper) error {
	for _, key := range []testKeys{volumeUp, powerBtn} {
		if err := pressBtnOnFWScreen(ctx, h, key); err != nil {
			return errors.Wrap(err, "failed to enable developer mode")
		}
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

func apResetUsingECCmd(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.RunECCommand(ctx, "apreset"); err != nil {
		return errors.Wrap(err, "failed to run apreset on ec console")
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}
