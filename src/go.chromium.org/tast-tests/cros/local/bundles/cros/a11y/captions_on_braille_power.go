// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CaptionsOnBraillePower,
		Desc: "Collect power metrics of using captions on braille",
		Contacts: []string{
			"chromeos-a11y-eng@google.com",
			"xiyuan@google.com",
		},
		BugComponent: "b:1272895",
		Timeout:      10*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome", "ondevice_speech"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Params: []testing.Param{{
			Name:    "captions_only",
			Fixture: a11y.PowerAshWithSoda,
			Val:     false,
		}, {
			Name:    "captions_on_braille",
			Fixture: a11y.PowerAshCaptionsOnBrailleWithSoda,
			Val:     true,
		}},
		Data: []string{
			"live_caption_power.html",
			"voice_en_long.ogg",
		},
	})
}

func CaptionsOnBraillePower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	const (
		pageTitle    = "Audio Playback For Live Caption Power Test"
		interval     = 5 * time.Second // Power metrics collect interval.
		testDuration = 5 * time.Minute
	)

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Enable ChromeVox and open the test page.
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr,
		tts.GoogleTTSEnUsVoice(), tts.GoogleTTSEngine(),
		server.URL+"/live_caption_power.html")
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	captionsOnBraille := s.Param().(bool)
	if !captionsOnBraille {
		s.Log("Turn on live captions via settings")
		if err := ossettings.ToggleLiveCaption(cr, tconn, true /*on=*/)(ctx); err != nil {
			s.Fatal("Failed to toggle on live caption: ", err)
		}
	} else {
		s.Log("Turn on Captions on Braille via keyboard command")
		if err := uiauto.Combine(
			"enable Captions on Braille",
			kb.AccelAction("Search+O"),
			kb.AccelAction("c"),
		)(ctx); err != nil {
			s.Fatal("Failed to press enable Captions on Braille: ", err)
		}
	}

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	// Cool down test device.
	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	pageRootWebArea := nodewith.Role(role.RootWebArea).Name(pageTitle)
	audioPlayButton := nodewith.Name("play").Role(role.Button).Ancestor(pageRootWebArea)
	audioPauseButton := nodewith.Name("pause").Role(role.Button).Ancestor(pageRootWebArea)
	liveCaptionBubble := nodewith.ClassName("CaptionBubbleLabel")

	if err := ui.DoDefaultUntil(audioPlayButton, ui.WithTimeout(3*time.Second).WaitUntilExists(audioPauseButton))(ctx); err != nil {
		s.Fatal("Failed to play the audio: ", err)
	}

	// Verify that live caption bubble appears in a short time.
	if err := uiauto.New(tconn).WithTimeout(10 * time.Second).WaitUntilExists(liveCaptionBubble)(ctx); err != nil {
		s.Fatal("Failed to wait for live caption bubble to show: ", err)
	}

	if captionsOnBraille {
		// Verify that CaptionsHandler is in live caption bubble.
		if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, liveCaptionBubble); err != nil {
			s.Fatal("Focus is not on live caption bubble: ", err)
		}
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// GoBigSleepLint: Keep the audio playing for the testDuration to measure the power usage.
	if err := testing.Sleep(ctx, testDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}
