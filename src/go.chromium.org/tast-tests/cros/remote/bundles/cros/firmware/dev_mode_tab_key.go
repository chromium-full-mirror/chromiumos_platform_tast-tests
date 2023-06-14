// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type noDebugInfoErr struct {
	*errors.E
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevModeTabKey,
		Desc:         "Verify that pressing the tab key on each firmware screen shows debug info",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Params: []testing.Param{{
			Name:              "chromebox",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebox)),
			Val:               true,
		}, {
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebox)),
			Val:               false,
		}},
		Timeout: 10 * time.Minute,
	})
}

func DevModeTabKey(ctx context.Context, s *testing.State) {
	ffIsChromebox := s.Param().(bool)
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	logPath := "/sys/firmware/log"

	// Save the firmware log file for upload to Stainless at the end of the test.
	defer func() {
		output, err := h.Reporter.CatFile(ctx, logPath)
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		destPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := ioutil.WriteFile(destPath, []byte(output), 0666); err != nil {
			s.Fatal("Failed to write firmware log: ", err)
		}
	}()

	waitDUTReconnect := func(ctx context.Context) error {
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx); err != nil {
			return errors.Wrap(err, "failed to connect DUT")
		}
		return nil
	}

	// Check which firmware screen the dut uses.
	// Retry if it wasn't found in the firmware log.
	var err error
	var mainFwScreenID firmware.FwScreenID
	const retry = 1
	for i := 0; i <= retry; i++ {
		mainFwScreenID, err = checkFwScreenType(ctx, h, logPath)
		if err == nil {
			break
		}
		if i == retry {
			// Don't reboot the dut at the last retry.
			continue
		}
		// Reset dut to ensure that we always start with a fresh firmware log.
		s.Log("Checking fw screen type failed. Reset DUT and retry")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
			s.Fatal("Failed to warm reset dut: ", err)
		}
		if err := waitDUTReconnect(ctx); err != nil {
			s.Fatal("Failed to reconnect DUT: ", err)
		}
	}
	if err != nil {
		s.Fatal("Failed to check fw screen type: ", err)
	}

	// Models released after May 16th have the "boot from external disk"
	// option always visible on the firmware menu.
	// Set 'crossystem dev_boot_usb' to 1 and 'crossystem dev_boot_altfw'
	// to 0, so that the menu layout becomes consistent on all machines.
	s.Log("Enabling dev_boot_usb and disabling dev_boot_altfw")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1", "dev_boot_altfw=0").Run(); err != nil {
		s.Fatal("Failed to set dev_boot_usb & dev_boot_altfw in crossystem: ", err)
	}
	for _, tc := range []struct {
		param     reporters.CrossystemParam
		expectVal string
	}{
		{reporters.CrossystemParamDevBootUsb, "1"},
		{reporters.CrossystemParamDevBootAltfw, "0"},
	} {
		val, err := h.Reporter.CrossystemParam(ctx, tc.param)
		if err != nil {
			s.Fatalf("Failed to get crossystem %s: %v", tc.param, err)
		}
		if val != tc.expectVal {
			s.Fatalf("Crossystem param %s was not set to %s", tc.param, tc.expectVal)
		}
	}

	// Power cycle the DUT to clear the firmware log, so that records prior
	// to this test are wiped.
	if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Start(); err != nil {
		s.Fatal("Failed to run poweroff cmd: ", err)
	}
	s.Log(ctx, "Checking for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		s.Fatal("Failed to get power state at G3: ", err)
	}
	s.Log("Sleeping for 5 seconds")
	// GoBigSleepLint: Sleeping for 5 seconds ensures power-off has
	// completely cleared the dut's firmware log.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for 5sec: ", err)
	}
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to power on DUT: ", err)
	}
	s.Logf("Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Starting to traverse all firmware screens")
	if err := blindlyNavigateThruMenu(ctx, h, mainFwScreenID, ffIsChromebox); err != nil {
		s.Fatal("Failed to traverse the firmware screen: ", err)
	}
	s.Log("Pressing Ctrl+D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		s.Fatal("Failed to make Ctrl+D press: ", err)
	}
	if err := waitDUTReconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect DUT: ", err)
	}

	// Grep texts relevant to firmware screens from
	// the firmware log file, and verify that they appeared
	// in the expected sequence.
	regs := `^(vboot_draw_|vb2ex_display_ui|ui_display).*screen=0x|VbDisplayDebugInfo`
	cmd := h.DUT.Conn().CommandContext(ctx, "grep", "-a", "-E", regs, logPath)
	stdout, err := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to run the command: ", err)
	}
	// Verify debug info screen on each fw screen.
	verifyScreenSeq, err := setVerifyScreenSequence(mainFwScreenID)
	if err != nil {
		s.Fatal("Failed to set fw screen verification sequence: ", err)
	}
	if ffIsChromebox {
		hasExternalDisplay, err := checkExternalDisplay(ctx, h)
		if err != nil {
			s.Fatal("Failed to check external display: ", err)
		}
		if !hasExternalDisplay {
			s.Fatal("Cannot find an external display connected to the chromebox")
		}
	}
	hasDebugInfoData := true
	for _, targetScreen := range verifyScreenSeq {
		var found, checkDebugInfoPage bool
		for scanner.Scan() {
			screenID, err := getScreenID(scanner.Text(), mainFwScreenID, checkDebugInfoPage)
			if err != nil {
				if _, ok := err.(*noDebugInfoErr); !ok {
					s.Fatal("Failed to get the fw screen id: ", err)
				}
				// If the debug info page wasn't found in the log file, the debug info
				// was probably printed in the top-left corner of the dut's screen. When
				// this is the case, check for the background screen, which would be the
				// same screen as the one that the dut has just traversed to. In the firmware
				// log, this screen would get recorded twice.
				s.Log("While scanning for firmware log: ", err.(*noDebugInfoErr))
				if screenID != targetScreen {
					s.Fatal("Unable to find background screen repeating when debug info page absent")
				}
				// In cases where debug info data are found floating at the top-left
				// corner, they are usually not recorded in the firmware log.
				hasDebugInfoData = false
			}
			if checkDebugInfoPage {
				break
			}
			if screenID == targetScreen {
				// Once the target screen gets verified, check for
				// the debug info screen id in the next line scanned.
				found = true
				checkDebugInfoPage = true
			}
		}
		if err := scanner.Err(); err != nil {
			s.Fatal("Failed to scan firmware log: ", err)
		}
		if found == false {
			s.Fatalf("Failed to find 0x%x", targetScreen)
		}
	}

	// Verify debug info data. Because the debug info content should be
	// consistent across all firmware screens. This was only verified once
	// based on the mainFwScreenId.
	if hasDebugInfoData {
		if err := checkDebugInfo(ctx, h, mainFwScreenID, logPath); err != nil {
			s.Fatal("Failed to check debug info data: ", err)
		}
	}
}

func checkFwScreenType(ctx context.Context, h *firmware.Helper, logPath string) (firmware.FwScreenID, error) {
	mainFwScreenID := firmware.Blank
	output, err := h.Reporter.CatFile(ctx, logPath)
	if err != nil {
		return mainFwScreenID, errors.Wrap(err, "failed to read firmware log")
	}

	for _, id := range []firmware.FwScreenID{
		firmware.DeveloperWarning,
		firmware.DeveloperWarningMenu,
		firmware.DeveloperMode,
	} {
		pattern := fmt.Sprintf("screen=0x%x", id)
		re := regexp.MustCompile(pattern)
		match := re.FindStringSubmatch(output)
		if len(match) == 1 {
			mainFwScreenID = id
			return mainFwScreenID, nil
		}
	}
	return mainFwScreenID, errors.New("unable to match any firmware screen type")
}

func getScreenID(log string, mainFwScreen firmware.FwScreenID, checkDebugInfoPage bool) (firmware.FwScreenID, error) {
	screenID := firmware.Blank
	// On some DUTs, such as astronaut/coral, debug info is shown in the top left corner.
	// On some other DUTs, such as jinlon/hatch, pressing <tab> would bring up a separate debug info page.
	// For the former case, return the id of the background firmware screen.
	// For the latter case, check for the VbDisplayDebugInfo or the screen=0x140 string.
	if strings.Contains(log, "VbDisplayDebugInfo") ||
		strings.Contains(log, "screen=0x140") {
		return firmware.DebugInfo, nil
	}
	var screenPrefix string
	if _, err := fmt.Sscanf(log, "%s screen=0x%x", &screenPrefix, &screenID); err != nil {
		return screenID, errors.Wrap(err, "failed to sscanf the screen prefix and id")
	}
	if checkDebugInfoPage {
		return screenID, &noDebugInfoErr{E: errors.New("did not find the debug info page, returning its background screen id")}
	}
	return screenID, nil
}

func checkDebugInfo(ctx context.Context, h *firmware.Helper, mainFwScreen firmware.FwScreenID, logPath string) error {
	debugInfo, err := h.Reporter.CatFile(ctx, logPath)
	if err != nil {
		return errors.Wrap(err, "failed to read firmware log")
	}

	regs := `HWID:(\n|.)*?kernel_subkey:[^\n\r]*`
	if mainFwScreen == firmware.DeveloperMode {
		regs = `HWID:(\n|.)*?TPM state:[^\n\r]*`
	}

	re := regexp.MustCompile(regs)
	if match := re.FindStringSubmatch(debugInfo); match == nil {
		return errors.New("failed to verify debug info data")
	}

	return nil
}

func blindlyNavigateThruMenu(ctx context.Context, h *firmware.Helper, mainFwScreenID firmware.FwScreenID, ffIsChromebox bool) error {
	var (
		upKey    = "arrow_up"
		downKey  = "arrow_down"
		spaceKey = " "
		enterKey = "<enter>"
		escKey   = "<esc>"
		tabKey   = "<tab>"
	)

	ecKBPress := func(key string) error {
		var err error
		switch key {
		case upKey, downKey:
			err = h.Servo.KeypressWithDuration(ctx, servo.KeypressControl(key), servo.DurTab)
		default:
			if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher || ffIsChromebox {
				err = h.Servo.PressUSBKey(ctx, key, servo.DurTab)
			} else {
				err = h.Servo.PressKey(ctx, key, servo.DurTab)
			}
		}
		if err != nil {
			return errors.Wrapf(err, "failed to press %s", key)
		}
		// GoBigSleepLint: Simulate a specific speed of key presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrap(err, "failed to wait for keypress delay")
		}
		return nil
	}

	nTimesTraverseSelect := func(n int, key string, selectOpt bool) error {
		testing.ContextLogf(ctx, "Pressing %s for %d times", key, n)
		for ; n > 0; n-- {
			if err := ecKBPress(key); err != nil {
				return errors.Wrapf(err, "failed to press %s", key)
			}
		}
		if selectOpt {
			if err := ecKBPress(enterKey); err != nil {
				return errors.Wrap(err, "failed to press ENTER key")
			}
		}
		return nil
	}
	/*
		We've found three major types of fw screens, as listed below with duts tested:
		1. detachable UI: kukui/kakadu
		2. white theme: sparky360, astronaut, jinlon
		3. dark theme: boten, eldrid
		Each one of these types requires a different traversing sequence, leading
		to other screens where a tab could bring up debug info.
	*/
	type traverse struct {
		hitTimes  int
		key       string
		selectOpt bool
	}
	var traverseSeq []traverse
	switch mainFwScreenID {
	case firmware.DeveloperWarningMenu:
		traverseSeq = []traverse{
			// Pressing tab key once displays debug info on the main menu.
			{1, tabKey, false},
			// Send tab key on Developer Options screen to display debug info.
			{3, upKey, true},
			{1, tabKey, false},
			// Send tab key on the Enable OS Verification screen to display debug info.
			{1, downKey, true},
			{1, upKey, true},
			{1, tabKey, false},
			// Send tab key on the Language screen to display debug info.
			{1, downKey, true},
			{1, downKey, true},
			{1, tabKey, false},
		}
	case firmware.DeveloperWarning:
		traverseSeq = []traverse{
			// Pressing tab key once displays debug info on the main menu.
			{1, tabKey, false},
			// Send tab key on the To-Norm screen to display debug info.
			{1, spaceKey, false},
			{1, tabKey, false},
			// Send tab key on the Language screen to display debug info.
			{1, escKey, false},
		}
	case firmware.DeveloperMode:
		traverseSeq = []traverse{
			// Pressing tab key once displays debug info on the main menu.
			{1, tabKey, false},
			// Send tab key on the Language screen to display debug info.
			{1, escKey, false},
			{2, upKey, true},
			{1, tabKey, false},
			// Send tab key on the Return To Secure Mode screen to display debug info.
			{2, escKey, false},
			{1, downKey, true},
			{1, tabKey, false},
			// Send tab key on the Advanced Options screen to display debug info.
			{2, escKey, false},
			{3, downKey, true},
			{1, tabKey, false},
			// Send tab key on the Firmware Log screen to display debug info.
			{1, escKey, false},
			{1, downKey, true},
			{1, tabKey, false},
		}
	}

	for _, seq := range traverseSeq {
		if err := nTimesTraverseSelect(seq.hitTimes, seq.key, seq.selectOpt); err != nil {
			return errors.Wrap(err, "failed to traverse fw screen")
		}
	}
	return nil
}

func setVerifyScreenSequence(mainFwScreenID firmware.FwScreenID) ([]firmware.FwScreenID, error) {
	switch mainFwScreenID {
	case firmware.DeveloperWarningMenu:
		return []firmware.FwScreenID{
			firmware.DeveloperWarningMenu,
			firmware.DeveloperMenu,
			firmware.DeveloperToNormMenu,
			firmware.LanguagesMenu,
		}, nil
	case firmware.DeveloperWarning:
		return []firmware.FwScreenID{
			firmware.DeveloperWarning,
			firmware.DeveloperToNorm,
		}, nil
	case firmware.DeveloperMode:
		return []firmware.FwScreenID{
			firmware.DeveloperMode,
			firmware.LanguageSelect,
			firmware.ReturnToSecureMode,
			firmware.AdvancedOptions,
			firmware.FirmwareLog,
		}, nil
	}
	return nil, errors.Errorf("Unable to identify the main dev screen: %q", mainFwScreenID)
}

func checkExternalDisplay(ctx context.Context, h *firmware.Helper) (bool, error) {
	if err := h.RequireRPCClient(ctx); err != nil {
		return false, errors.Wrap(err, "failed to open RPC client")
	}
	fs := dutfs.NewClient(h.RPCClient.Conn)
	drmPath := "/sys/class/drm"
	cardDirs, err := fs.ReadDir(ctx, drmPath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read the card dirs in %s", drmPath)
	}
	// DP displays show up as card*-DP-*
	// HDMI displays show up as card*-HDMI-*
	cardMatch := regexp.MustCompile(`^card[0-9]-(DP|HDMI).*[0-9]$`)
	for _, dir := range cardDirs {
		cardDir := dir.Name()
		if cardMatch.MatchString(cardDir) {
			cardConnected, err := fs.ReadFile(ctx, path.Join(drmPath, cardDir, "status"))
			if err != nil {
				return false, errors.Wrap(err, "failed to read status")
			}
			if strings.HasPrefix(string(cardConnected), "connected") {
				return true, nil
			}
		}
	}
	// No external display connected.
	return false, nil
}
