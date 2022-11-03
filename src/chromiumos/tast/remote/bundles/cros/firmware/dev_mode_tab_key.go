// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

/*
fwScreenId contains the id for each individual fw screen.
These ids were found from the depthcharge repo:
'src/vboot/ui.h' & 'src/drivers/video/display.h'
*/
type fwScreenID int

// Below are the ids representing different fw screens.
const (
	blank            fwScreenID = 0x0
	developerWarning fwScreenID = 0x101
	developerToNorm  fwScreenID = 0x205

	developerWarningMenu fwScreenID = 0x20a
	developerMenu        fwScreenID = 0x20b
	developerToNormMenu  fwScreenID = 0x20e
	languagesMenu        fwScreenID = 0x20f

	advancedOptions    fwScreenID = 0x120
	languageSelect     fwScreenID = 0x130
	debugInfo          fwScreenID = 0x140
	firmwareLog        fwScreenID = 0x150
	developerMode      fwScreenID = 0x300
	returnToSecureMode fwScreenID = 0x310
)

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
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      10 * time.Minute,
	})
}

func DevModeTabKey(ctx context.Context, s *testing.State) {
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

	// Check which firmware screen the dut uses.
	mainFwScreenID, err := checkFwScreenType(ctx, h, logPath)
	if err != nil {
		s.Fatal("Failed to check fw screen type: ", err)
	}

	// Disable dev_boot_usb and dev_boot_altfw to remove the associated options
	// on the dev fw screen, and to ensure consistency in the traverse sequence.
	s.Log("Disabling dev_boot_usb & dev_boot_altfw")
	if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0", "dev_boot_altfw=0").Run(); err != nil {
		s.Fatal("Failed to disable dev_boot_usb & dev_boot_altfw: ", err)
	}

	// Power cycle the DUT to clear the firmware log, so that records prior
	// to this test are wiped.
	if err := h.DUT.Conn().CommandContext(ctx, "poweroff").Run(); err != nil {
		s.Fatal("Failed to run poweroff cmd: ", err)
	}
	s.Log(ctx, "Checking for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		s.Fatal("Failed to get power state at G3: ", err)
	}
	// Sleeping for 5 seconds ensures the DUT's power completely off.
	s.Log("Sleeping for 5 seconds")
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for 5sec: ", err)
	}
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to power on DUT: ", err)
	}
	s.Logf("Sleeping for %s (FirmwareScreen)", h.Config.FirmwareScreen)
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Starting to traverse all firmware screens")
	if err := blindlyNavigateThruMenu(ctx, h, mainFwScreenID); err != nil {
		s.Fatal("Failed to traverse the firmware screen: ", err)
	}
	s.Log("Pressing Ctrl+D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		s.Fatal("Failed to make Ctrl+D press: ", err)
	}
	s.Log("Waiting for the DUT to reconnect")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to connect DUT: ", err)
	}

	// Grep texts relevant to firmware screens from
	// the firmware log file, and verify that they appeared
	// in the expected sequence.
	regs := `^(vboot_draw_|vb2ex_display_ui).*screen=0x|VbDisplayDebugInfo`
	cmd := h.DUT.Conn().CommandContext(ctx, "grep", "-E", regs, logPath)
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
	for _, targetScreen := range verifyScreenSeq {
		var found, checkDebugInfoPage bool
		for scanner.Scan() {
			screenID, err := getScreenID(scanner.Text(), mainFwScreenID, checkDebugInfoPage)
			if err != nil {
				s.Fatal("Failed to get the fw screen id: ", err)
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
	if err := checkDebugInfo(ctx, h, mainFwScreenID, logPath); err != nil {
		s.Fatal("Failed to check debug info data: ", err)
	}
}

func checkFwScreenType(ctx context.Context, h *firmware.Helper, logPath string) (fwScreenID, error) {
	mainFwScreenID := blank
	output, err := h.Reporter.CatFile(ctx, logPath)
	if err != nil {
		return mainFwScreenID, errors.Wrap(err, "failed to read firmware log")
	}

	for _, id := range []fwScreenID{
		developerWarning,
		developerWarningMenu,
		developerMode,
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

func getScreenID(log string, mainFwScreen fwScreenID, checkDebugInfoPage bool) (fwScreenID, error) {
	screenID := blank
	if checkDebugInfoPage {
		// On some DUTs, such as astronaut/coral, debug info is shown in the top left corner.
		// On some other DUTs, such as jinlon/hatch, pressing <tab> would bring up a separate debug info page.
		// For the former case, check for the VbDisplayDebugInfo string. For the latter case, verify
		// vboot_draw_ui at the developer warning screen.
		if strings.Contains(log, "VbDisplayDebugInfo") ||
			strings.Contains(log, "screen=0x140") ||
			((mainFwScreen == developerWarning) && (strings.HasPrefix(log, "vboot_draw_ui:"))) {
			return debugInfo, nil
		}
		return screenID, errors.New("Unable to find the debug info page")
	}
	var screenPrefix string
	if _, err := fmt.Sscanf(log, "%s screen=0x%x", &screenPrefix, &screenID); err != nil {
		return screenID, errors.Wrap(err, "failed to sscanf the screen prefix and id")
	}
	return screenID, nil
}

func checkDebugInfo(ctx context.Context, h *firmware.Helper, mainFwScreen fwScreenID, logPath string) error {
	// Grep for HWID information, which is usually the first line
	// displayed on the debug info page, and check that debug info
	// data is available.
	output, err := h.DUT.Conn().CommandContext(ctx, "grep", "-m1", "-A20", `HWID:[^\n\r]*`, logPath).Output()
	if err != nil {
		// In cases where debug info data are found floating at the top-left
		// corner, they are usually not recorded in the firmware log. Don't fail on such cases.
		if mainFwScreen == developerWarning {
			testing.ContextLog(ctx, "Skip verifying for debug info data found floating at the top-left corner")
			return nil
		}
		return errors.Wrap(err, "failed to capture debug info data")
	}
	debugInfo := string(output)

	regs := `HWID:(\n|.)*?kernel_subkey:[^\n\r]*`
	if mainFwScreen == developerMode {
		regs = `HWID:(\n|.)*?TPM state:[^\n\r]*`
	}

	re := regexp.MustCompile(regs)
	if match := re.FindStringSubmatch(debugInfo); match == nil {
		return errors.New("failed to verify debug info data")
	}

	return nil
}

func blindlyNavigateThruMenu(ctx context.Context, h *firmware.Helper, mainFwScreenID fwScreenID) error {
	var (
		volUp    = "volumeUp"
		volDown  = "volumeDown"
		upKey    = "<up>"
		downKey  = "<down>"
		spaceKey = " "
		enterKey = "<enter>"
		escKey   = "<esc>"
		tabKey   = "<tab>"
	)

	nTimesTraverseSelect := func(n int, key string, selectOpt bool) error {
		testing.ContextLogf(ctx, "Pressing %s for %d times", key, n)
		for ; n > 0; n-- {
			var err error
			switch key {
			case volUp:
				err = h.Servo.SetInt(ctx, servo.VolumeUpHold, 100)
			case volDown:
				err = h.Servo.SetInt(ctx, servo.VolumeDownHold, 100)
			default:
				err = h.Servo.PressKey(ctx, key, servo.DurTab)
			}
			if err != nil {
				return errors.Wrapf(err, "failed to press %s", key)
			}
		}
		if selectOpt {
			if mainFwScreenID == developerWarningMenu {
				testing.ContextLog(ctx, "Pressing power button")
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
					return errors.Wrap(err, "failed to press power button")
				}
			} else {
				testing.ContextLog(ctx, "Pressing ENTER")
				if err := h.Servo.PressKey(ctx, enterKey, servo.DurTab); err != nil {
					return errors.Wrap(err, "failed to press ENTER key")
				}
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
	case developerWarningMenu:
		traverseSeq = []traverse{
			// Pressing tab key once displays debug info on the main menu.
			{1, tabKey, false},
			// Send tab key on Developer Options screen to display debug info.
			{3, volUp, true},
			{1, tabKey, false},
			// Send tab key on the Enable OS Verification screen to display debug info.
			{1, volDown, true},
			{1, volUp, true},
			{1, tabKey, false},
			// Send tab key on the Language screen to display debug info.
			{1, volDown, true},
			{1, volDown, true},
			{1, tabKey, false},
		}
	case developerWarning:
		traverseSeq = []traverse{
			// Pressing tab key once displays debug info on the main menu.
			{1, tabKey, false},
			// Send tab key on the To-Norm screen to display debug info.
			{1, spaceKey, false},
			{1, tabKey, false},
			// Send tab key on the Language screen to display debug info.
			{1, escKey, false},
		}
	case developerMode:
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
			{2, downKey, true},
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

func setVerifyScreenSequence(mainFwScreenID fwScreenID) ([]fwScreenID, error) {
	switch mainFwScreenID {
	case developerWarningMenu:
		return []fwScreenID{
			developerWarningMenu,
			developerMenu,
			developerToNormMenu,
			languagesMenu,
		}, nil
	case developerWarning:
		return []fwScreenID{
			developerWarning,
			developerToNorm,
		}, nil
	case developerMode:
		return []fwScreenID{
			developerMode,
			languageSelect,
			returnToSecureMode,
			advancedOptions,
			firmwareLog,
		}, nil
	}
	return nil, errors.Errorf("Unable to identify the main dev screen: %q", mainFwScreenID)
}
