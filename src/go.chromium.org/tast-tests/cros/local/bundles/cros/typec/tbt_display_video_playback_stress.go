// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/typec/setup"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type videoStressTestParams struct {
	minutes int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TBTDisplayVideoPlaybackStress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Youtube video playback on TBT display for long duration ",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:typec"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"test_config.json", "testcert.p12"},
		Vars:         []string{"typec.dutTbtPort", "typec.cSwitchPort", "typec.domainIP"},
		Fixture:      "chromeLoggedInThunderbolt",
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), setup.ThunderboltSupportedDevices()),
		Params: []testing.Param{{
			Name:    "quick",
			Val:     videoStressTestParams{minutes: 4},
			Timeout: 15 * time.Minute,
		}, {
			Name:      "bronze",
			Val:       videoStressTestParams{minutes: 2 * 60}, // 2 hours.
			Timeout:   135 * time.Minute,
			ExtraAttr: []string{"group:intel-tbt4-display"},
		}, {
			Name:    "silver",
			Val:     videoStressTestParams{minutes: 4 * 60}, // 4 hours.
			Timeout: 255 * time.Minute,
		}, {
			Name:    "gold",
			Val:     videoStressTestParams{minutes: 6 * 60}, // 6 hours.
			Timeout: 375 * time.Minute,
		},
		}})
}

// TBTDisplayVideoPlaybackStress requires the following H/W topology to run.
// Here, TBT stands for Thunderbolt.
// DUT ---> C-Switch(device that performs hot plug-unplug) ---> TBT Dock Station ---> TBT Display.
func TBTDisplayVideoPlaybackStress(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	duration := s.Param().(videoStressTestParams)

	const (
		expectedAudioNode = "HDMI"
		extendedDisplay   = true
		playingState      = 1 // Playing state of the Youtube player.
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	// Config file which contains expected values of USB4/TBT parameters.
	const testConfig = "test_config.json"
	// TBT port ID in the DUT.
	tbtPort := s.RequiredVar("typec.dutTbtPort")
	// cswitch port ID.
	cSwitchON := s.RequiredVar("typec.cSwitchPort")
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("typec.domainIP")

	if err := typecutils.EnablePeripheralDataAccess(ctx, s.DataPath("testcert.p12")); err != nil {
		s.Fatal("Failed to enable peripheral data access setting: ", err)
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Read json config file.
	jsonData, err := ioutil.ReadFile(s.DataPath(testConfig))
	if err != nil {
		s.Fatal("Failed to read response data: ", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	// Checking for TBT config data.
	deviceVal, ok := data["TBT"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to found TBT config data in JSON file")
	}

	// Create C-Switch session that performs hot plug-unplug on TBT/USB4 device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create sessionID: ", err)
	}

	const cSwitchOFF = "0"
	defer func(ctx context.Context) {
		s.Log("Cleanup")
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
		}
		if err := cswitch.CloseSession(cleanupCtx, sessionID, domainIP); err != nil {
			s.Log("Failed to close sessionID: ", err)
		}
	}(cleanupCtx)

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}

	if _, err := cswitch.IsDeviceEnumerated(ctx, deviceVal["device_name"].(string), tbtPort); err != nil {
		s.Fatal("Failed to enumerate the TBT device: ", err)
	}

	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      usbutils.TBTDisplay,
	}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external TBT display: ", err)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Error("Failed to create Cras object: ", err)
	}

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=uu_B4ywAhOM",
		Title:   "8 Hours Beautiful World from a Bird’s Eye View 4K / Relaxation Time",
		Quality: "2160p",
	}

	// Create an instance of YtWeb to perform actions on youtube web.
	ytbWeb := youtube.NewYtWeb(cr.Browser(), tconn, kb, extendedDisplay, ui, uiHandler)
	defer ytbWeb.Close(cleanupCtx)
	defer cuj.SwitchWindowToDisplay(cleanupCtx, tconn, kb, !extendedDisplay)(ctx)

	if err := ytbWeb.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %s: %v", videoSource.URL, err)
	}

	if err = ytbWeb.Play()(ctx); err != nil {
		s.Fatal("Failed to play the video: ", err)
	}

	// Setting the active node to 'HDMI' if default node is set to some other node.
	if err := cras.SetActiveNodeByType(ctx, expectedAudioNode); err != nil {
		s.Fatalf("Failed to select active device %q: %v", expectedAudioNode, err)
	}
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}
	if deviceType != expectedAudioNode {
		s.Fatalf("Failed to set the audio node type: got %q; want %q", deviceType, expectedAudioNode)
	}

	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}

	if deviceName != devName {
		s.Fatalf("Failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
	}

	// videoPlaying verifies whether youtube video is playing or not.
	videoPlaying := func() error {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var playerState int
			if err := ytbWeb.YtWebConn().Eval(ctx, `document.getElementById('movie_player').getPlayerState()`, &playerState); err != nil {
				return errors.Wrap(err, "failed to get youtube player state")
			}
			if playerState != playingState {
				return errors.New("youtube video is not playing")
			}
			s.Log("Youtube video is playing")
			return nil
		}, &testing.PollOptions{Timeout: 40 * time.Second, Interval: 2 * time.Second}); err != nil {
			return err
		}
		return nil
	}

	startTime := time.Now().Unix()
	endTime := float64(duration.minutes * 60)

	if err := testing.Poll(ctx, func(c context.Context) error {
		if err := videoPlaying(); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to play video"))
		}
		devName, err = crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			return errors.Wrap(err, "failed to detect running output device")
		}
		if deviceName != devName {
			return errors.Errorf("failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
		}
		if err := ytbWeb.PerformFrameDropsTest(ctx); err != nil {
			return errors.Wrap(err, "failed to play video without frame drops")
		}
		elapsed := float64(time.Now().Unix() - startTime)
		if elapsed < endTime {
			s.Logf("Audio is routing to %s, test remaining time: %f/%f sec", expectedAudioNode, elapsed, endTime)
			return errors.New("audio is routing")
		}
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Minute, Timeout: time.Duration(duration.minutes+10) * time.Minute}); err != nil {
		s.Fatalf("Failed to play Youtube through %s device: %v", expectedAudioNode, err)
	}
}
