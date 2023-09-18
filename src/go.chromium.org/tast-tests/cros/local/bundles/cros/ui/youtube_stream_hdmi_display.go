// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var outputNodeHDMIRe = regexp.MustCompile(`yes.*HDMI.*\d\*`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         YoutubeStreamHDMIDisplay,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verifies Youtube stream with 4K display and checks display functionalities on HDMI monitor connected on USB type-C port",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-cswitch-set1"},
		Data:         []string{"testcert.p12"},
		Vars:         []string{"ui.cSwitchPort", "ui.domainIP"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

// YoutubeStreamHDMIDisplay test requires the following H/W topology to run.
// DUT ------> C-Switch(device that performs hot plug-unplug) ----> External Type-C HDMI display.
func YoutubeStreamHDMIDisplay(ctx context.Context, s *testing.State) {
	// Give 5 seconds to cleanup other resources.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}

	initialBrightness, err := systemBrightness(ctx)
	if err != nil {
		s.Fatal("Failed to get initial brightness: ", err)
	}

	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=LXb3EKWsInQ",
		Title:   "COSTA RICA IN 4K 60fps HDR (ULTRA HD)",
		Quality: "1440p60",
	}

	// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
	cswitchVar := "1"
	if cswitchON, ok := s.Var("ui.cSwitchPort"); ok {
		cswitchVar = cswitchON
	}
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("ui.domainIP")

	// Create C-Switch session that performs hot plug-unplug on USB4 device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create session: ", err)
	}

	const cSwitchOFF = "0"
	defer func(ctx context.Context) {
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}

		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Log("Failed to close session: ", err)
		}
	}(cleanupCtx)

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cswitchVar, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}

	if err := waitForSettingsApp(ctx, tconn); err != nil {
		s.Fatal("Failed to find the settings app in the available Chrome apps: ", err)
	}

	if err := typecutils.CheckDisplayInfo(ctx, true, false); err != nil {
		s.Fatal("Failed to check display info: ", err)
	}

	cui := uiauto.New(tconn)
	info, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	if err := typecutils.SetDisplayResolution(ctx, tconn, &info[1], 3840, 2160, cr); err != nil {
		s.Fatal("Failed to change resolution: ", err)
	}

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, true, cui, uiHandler)
	if err := videoApp.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %s: %v", videoSource.URL, err)
	}
	defer videoApp.Close(cleanupCtx)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	deviceName, deviceType, err := setAudioNodeAsHDMI(ctx, cras)
	if err != nil {
		s.Fatal("Failed to set HDMI as active audio node: ", err)
	}

	if err := verifyHDMIInCrasTest(ctx); err != nil {
		s.Fatal("Failed to verify HDMI as output audio node in cras_test_client command: ", err)
	}

	if err := typecutils.VerifyAudioRoute(ctx, deviceName); err != nil {
		s.Fatalf("Failed to verify audio routing through %q: %v", deviceType, err)
	}

	if err := youtubePlayerFunctionalities(ctx, kb, cui, tconn); err != nil {
		s.Fatal("Failed to perform youtube player functionalities: ", err)
	}

	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		s.Fatal("Failed to obtain the top-row layout: ", err)
	}

	if err := decreaseBrightness(ctx, topRow, kb); err != nil {
		s.Fatal("Failed to decreare brightness percent to zero: ", err)
	}

	// Increasing brightness level with on-board keyboard key press as cleanup.
	defer increaseBrightness(cleanupCtx, topRow, kb, initialBrightness)
}

// waitForChangesInBrightness waits for change in brightness value while calling doBrightnessChange function.
// doBrightnessChange does brightness value change with keyboard BrightnessUp/BrightnessDown keypress.
func waitForChangesInBrightness(ctx context.Context, doBrightnessChange func() error) (float64, error) {
	brightness, err := systemBrightness(ctx)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to get system brightness")
	}
	if err := doBrightnessChange(); err != nil {
		return 0.0, errors.Wrap(err, "failed in calling doBrightnessChange function")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		newBrightness, err := systemBrightness(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get brightness after doBrightnessChange function is called"))
		}
		if brightness == newBrightness {
			return errors.New("brightness not changed")
		}
		brightness = newBrightness
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return 0.0, errors.Wrap(err, "failed to wait for brightness change")
	}
	//  returns change in brightness value.
	return math.Round(brightness*100) / 100, nil
}

// systemBrightness gets the current brightness of the system.
func systemBrightness(ctx context.Context) (float64, error) {
	out, err := testexec.CommandContext(ctx, "backlight_tool", "--get_brightness_percent").Output()
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to execute brightness command")
	}
	b, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to parse string into float64")
	}
	// returns system brightness value.
	return math.Round(b*100) / 100, nil
}

// setSystemBrightness sets the brightness of the system.
func setSystemBrightness(ctx context.Context, percent float64) error {
	if err := testexec.CommandContext(ctx, "backlight_tool", fmt.Sprintf("--set_brightness_percent=%f", percent)).Run(); err != nil {
		return errors.Wrapf(err, "failed to set %f%% brightness", percent)
	}
	return nil
}

// decreaseBrightness performs brightness decrease with keyboard keypress.
func decreaseBrightness(ctx context.Context, topRow *input.TopRowLayout, kb *input.KeyboardEventWriter) error {
	for {
		preBrightness, err := systemBrightness(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get brightness")
		}
		decBrightness, err := waitForChangesInBrightness(ctx, func() error {
			return kb.Accel(ctx, topRow.BrightnessDown)
		})
		if err != nil {
			return errors.Wrap(err, "failed to change brightness after pressing 'BrightnessDown'")
		}
		if decBrightness > preBrightness {
			return errors.Wrap(err, "failed to decrease the brightness")
		}
		if decBrightness == 0.00 {
			break
		}
	}
	return nil
}

// increaseBrightness performs brightness increase with keyboard keypress.
func increaseBrightness(ctx context.Context, topRow *input.TopRowLayout, kb *input.KeyboardEventWriter, initialBrightness float64) error {
	for {
		preBrightness, err := systemBrightness(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get brightness")
		}
		incBrightness, err := waitForChangesInBrightness(ctx, func() error {
			return kb.Accel(ctx, topRow.BrightnessUp)
		})
		if err != nil {
			return errors.Wrap(err, "failed to change brightness after pressing 'BrightnessUp'")
		}
		if incBrightness <= preBrightness {
			return errors.Wrap(err, "failed to increase the brightness")
		}
		if incBrightness == initialBrightness {
			break
		}
	}
	return nil
}

// waitForExternalDisplayName will returns connected external display name.
func waitForExternalDisplayName(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	var displayName string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		displayInfo, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get external display info")
		}
		if len(displayInfo) < 2 {
			return errors.New("failed please connect external 4K monitor")
		}
		displayName = displayInfo[1].Name
		if displayName == "" {
			return errors.New("external display name is empty")
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to get external display info")
	}
	return displayName, nil
}

// waitForSettingsApp will check for availability of settings app.
func waitForSettingsApp(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		capps, err := ash.ChromeApps(ctx, tconn)
		if err != nil {
			return testing.PollBreak(err)
		}
		for _, app := range capps {
			if app.AppID == apps.Settings.ID {
				return nil
			}
		}
		return errors.New("settings app not yet found in available Chrome apps")
	}, nil)
}

// setAudioNodeAsHDMI performs setting of active audio node to external HDMI.
func setAudioNodeAsHDMI(ctx context.Context, cras *audio.Cras) (string, string, error) {
	const expectedAudioOuputNode = "HDMI"
	// Get current audio output device info.
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get the selected audio device")
	}

	if deviceType != expectedAudioOuputNode {
		if err := cras.SetActiveNodeByType(ctx, expectedAudioOuputNode); err != nil {
			return "", "", errors.Wrapf(err, "failed to select active device %s", expectedAudioOuputNode)
		}
		deviceName, deviceType, err = cras.SelectedOutputDevice(ctx)
		if err != nil {
			return "", "", errors.Wrap(err, "failed to get the selected audio device")
		}
		if deviceType != expectedAudioOuputNode {
			return "", "", errors.Errorf("failed to set the audio node type: got %q; want %q", deviceType, expectedAudioOuputNode)
		}
	}
	return deviceName, deviceType, nil
}

// verifyHDMIInCrasTest verifies connected HDMI is detected in cras_test_client.
func verifyHDMIInCrasTest(ctx context.Context) error {
	out, err := testexec.CommandContext(ctx, "cras_test_client").Output()
	if err != nil {
		return errors.Wrap(err, "failed to exceute cras_test_client command")
	}
	if !outputNodeHDMIRe.Match(out) {
		return errors.New("failed to select HDMI as output audio node")
	}
	return nil
}

// waitForMirrorModeSwitch verifies mirror mode switching.
func waitForMirrorModeSwitch(ctx context.Context, tconn *chrome.TestConn) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		info, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get extrenal display info")
		}
		if info[0].MirroringSourceID == "" {
			return errors.New("DUT is not in mirror mode")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 250 * time.Millisecond})
}

// youtubePlayerFunctionalities performs various youtube player functionalities.
func youtubePlayerFunctionalities(ctx context.Context, kb *input.KeyboardEventWriter, cui *uiauto.Context, tconn *chrome.TestConn) error {
	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}
	if err := kb.Accel(ctx, "f"); err != nil {
		return errors.Wrap(err, "failed to press f key to enter fullscreen")
	}

	exitFullScreenText := nodewith.Name(`Exit full screen (f)`).Role(role.Button)
	if err := cui.WaitUntilExists(exitFullScreenText)(ctx); err != nil {
		return errors.Wrap(err, "failed to check the existence of 'Exit full screen' to valiadte the full screen mode")
	}

	if err := kb.Accel(ctx, "f"); err != nil {
		return errors.Wrap(err, "failed to press f key to exit fullscreen")
	}

	fullScreenText := nodewith.Name(`Full screen (f)`).Role(role.Button)
	if err := cui.WaitUntilExists(fullScreenText)(ctx); err != nil {
		return errors.Wrap(err, "failed to check the existence of 'Full screen' to valiadte exit of full screen")
	}

	if err := kb.Accel(ctx, "alt+-"); err != nil {
		return errors.Wrap(err, "failed to press alt+- key to minimize the window")
	}
	muteButtonText := nodewith.Name(`Mute (m)`).Role(role.Button)
	if err := cui.WaitUntilExists(muteButtonText)(ctx); err == nil {
		return errors.Wrap(err, "failed to check the existence of Mute (m) to valiadte whether screen is minimized or not")
	}

	if err := kb.Accel(ctx, "alt+-"); err != nil {
		return errors.Wrap(err, "failed to press alt+- key to normalize the window back from minimize")
	}
	if err := cui.WaitUntilExists(muteButtonText)(ctx); err != nil {
		return errors.Wrap(err, "failed due to existence of Mute (m) after normalizing window mode from minimize")
	}

	if err := kb.Accel(ctx, "alt+="); err != nil {
		return errors.Wrap(err, "failed to press alt+= key to maximize the window")
	}

	maximizeButton := nodewith.Name("Maximize").Role(role.Button)
	if err := cui.WaitUntilExists(maximizeButton)(ctx); err == nil {
		return errors.Wrap(err, "failed due to existence of Maximize button after maximizing window")
	}

	if err := kb.Accel(ctx, "alt+="); err != nil {
		return errors.Wrap(err, "failed to press alt+= key to normalize the window back from maximize")
	}

	if err := cui.WaitUntilExists(maximizeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to check the existence of Maximize button after normalizing window")
	}

	if err := kb.Accel(ctx, topRow.SelectTask); err != nil {
		return errors.Wrap(err, "failed to press scale button")
	}

	deskButtonText := nodewith.Name("Desk 1").Role(role.StaticText).First()
	if err := cui.WaitUntilExists(deskButtonText)(ctx); err != nil {
		return errors.Wrap(err, "failed to check the existence of Desk 1 element to validate window mode")
	}

	if err := kb.Accel(ctx, topRow.SelectTask); err != nil {
		return errors.Wrap(err, "failed to press scale button")
	}

	if err := cui.WithTimeout(10 * time.Second).WaitUntilGone(deskButtonText)(ctx); err != nil {
		return errors.Wrap(err, "failed due to existence of Desk 1 element to validate normal mode")
	}

	if err := typecutils.SetMirrorDisplay(ctx, tconn, true); err != nil {
		return errors.Wrap(err, "failed to press ctrl+fullscreen")
	}

	if err := waitForMirrorModeSwitch(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to verify mirror mode switch")
	}
	return nil
}
