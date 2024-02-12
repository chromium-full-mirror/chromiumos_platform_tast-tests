// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LiveCaptionPower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Ash is only a platform dependency.
		Desc:         "Collect power metrics of using live caption",
		Contacts: []string{
			"ml-service-team@google.com",
			"amoylan@chromium.org",
			"chenjih@google.com",
		},
		Fixture: "powerAsh",
		// Software > Machine Intelligence > libsoda & ChromeOS Live Caption.
		BugComponent: "b:1116342",
		Timeout:      10*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome", "ondevice_speech", "camera_feature_effects"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Params: []testing.Param{{
			Name: "no_effects",
			Val:  false,
		}, {
			Name: "live_caption",
			Val:  true,
		}},
		Data: []string{
			"live_caption_power.html",
			"voice_en_long.ogg",
		},
	})
}

func LiveCaptionPower(ctx context.Context, s *testing.State) {
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

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	useLiveCaption := s.Param().(bool)
	if err := ossettings.ToggleLiveCaption(cr, tconn, useLiveCaption)(ctx); err != nil {
		s.Fatal("Failed to toggle on live caption: ", err)
	}

	if useLiveCaption {
		// Wait until dlc libsoda and libsoda-model-en-us are installed.
		if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
			s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
		}
	}

	// Open the test page and play the audio.
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/live_caption_power.html")
	if err != nil {
		s.Fatal("Failed to open test web page: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	// Cool down test device.
	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	pageRootWebArea := nodewith.Role(role.RootWebArea).Name(pageTitle)
	audioPlayButton := nodewith.Name("play").Role(role.Button).Ancestor(pageRootWebArea)
	audioPauseButton := nodewith.Name("pause").Role(role.Button).Ancestor(pageRootWebArea)
	liveCaptionBubble := nodewith.ClassName("CaptionBubbleFrameView")

	if err := ui.DoDefaultUntil(audioPlayButton, ui.WithTimeout(3*time.Second).WaitUntilExists(audioPauseButton))(ctx); err != nil {
		s.Fatal("Failed to play the audio: ", err)
	}

	// Verify that live caption bubble appears if feature is enabled.
	if useLiveCaption {
		if err := ui.WaitUntilExists(liveCaptionBubble)(ctx); err != nil {
			s.Fatal("Failed to wait for live caption bubble to show: ", err)
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
}
