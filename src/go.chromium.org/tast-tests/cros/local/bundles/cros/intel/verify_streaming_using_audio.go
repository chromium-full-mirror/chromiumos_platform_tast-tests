// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyStreamingUsingAudio,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Audio - Streaming using audio streaming apps",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

func VerifyStreamingUsingAudio(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}
	// Create test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cui := uiauto.New(tconn)

	// Here open yoputube and play the video.
	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=LXb3EKWsInQ",
		Title:   "COSTA RICA IN 4K 60fps HDR (ULTRA HD)",
		Quality: "1440p60",
	}

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, false, cui, uiHandler)
	if err := videoApp.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %s: %v", videoSource.URL, err)
	}
	defer videoApp.Close(cleanupCtx)

	if err := videoApp.PauseAndPlayVideo(ctx); err != nil {
		s.Fatal("Failed to pause and play video: ", err)
	}
	conn := videoApp.YtWebConn()
	var currentTime float64

	var (
		JsScriptNxt  = `document.getElementsByTagName("video")[0].currentTime=100`
		JsScriptPrev = `document.getElementsByTagName("video")[0].currentTime=0`
		JsScriptCurr = `document.getElementsByTagName("video")[0].currentTime`
	)

	if err := conn.Eval(ctx, JsScriptNxt, nil); err != nil {
		s.Fatal("Failed to forward video: ", err)
	}
	if err := conn.Eval(ctx, JsScriptCurr, &currentTime); err != nil {
		s.Fatal("Failed to fetch video current time: ", err)
	}
	if currentTime < 100 {
		s.Fatal("Failed to verify video forward")
	}

	if err := conn.Eval(ctx, JsScriptPrev, nil); err != nil {
		s.Fatal("Failed to rewind video: ", err)
	}
	if err := conn.Eval(ctx, JsScriptCurr, &currentTime); err != nil {
		s.Fatal("Failed to fetch video current time: ", err)
	}
	if currentTime > 5 {
		s.Fatal("Failed to move previous")
	}

	const expectedAudioNode = "INTERNAL_SPEAKER"
	if err := verifyAudioRouting(ctx, expectedAudioNode); err != nil {
		s.Fatal("Failed to verify audio routing: ", err)
	}
}

// verifyAudioRouting function verifies whether audio is routing or not on specified audio device.
func verifyAudioRouting(ctx context.Context, expectedAudioNode string) error {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create cras object")
	}
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the selected audio device")
	}
	if deviceType != expectedAudioNode {
		if err := cras.SetActiveNodeByType(ctx, expectedAudioNode); err != nil {
			return errors.Wrapf(err, "failed to select active device %s", expectedAudioNode)
		}
		deviceName, deviceType, err = cras.SelectedOutputDevice(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the selected audio device")
		}
		if deviceType != expectedAudioNode {
			return errors.Wrapf(err, "failed to set the audio node type: got %q; want %q", deviceType, expectedAudioNode)
		}
	}
	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		return errors.Wrap(err, "failed to detect running output device")
	}
	if deviceName != devName {
		return errors.Errorf("failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
	}
	return nil
}
