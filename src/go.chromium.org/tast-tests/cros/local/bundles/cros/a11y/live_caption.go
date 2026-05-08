// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type liveCaptionParams struct {
	fieldTrialConfig      int    // One of chrome.FieldTrialConfig{Enabled,Disabled}.
	liveTranslateLanguage string // If non empty, the language to set translation to.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LiveCaption,
		Desc: "Checks live caption works",
		Contacts: []string{
			"ml-service-team@google.com",
			"amoylan@chromium.org",
		},
		// Software > Machine Intelligence > libsoda & ChromeOS Live Caption
		BugComponent: "b:1116342",
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "ondevice_speech"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{
			{
				Name: "fieldtrial_testing_config_off",
				Val:  liveCaptionParams{chrome.FieldTrialConfigDisable, ""},
			}, {
				Name: "fieldtrial_testing_config_on",
				Val:  liveCaptionParams{chrome.FieldTrialConfigEnable, ""},
			}},
		Data: []string{
			"live_caption.html",
			"voice_en_hello.wav",
		},
	})
}

func LiveCaption(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	params := s.Param().(liveCaptionParams)

	cr, err := chrome.New(ctx,
		chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"), // Allow media autoplay.
		chrome.EnableFeatures("OnDeviceSpeechRecognition", "LayoutMediaNGContainer", "LiveTranslate"),
		chrome.FieldTrialConfig(params.fieldTrialConfig),
		chrome.DisableFeatures("CrosSodaConchLanguages"),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	if err := ossettings.ToggleLiveCaption(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to toggle on live caption: ", err)
	}

	// if asked to test live translate, do so.
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "live translate ui tree")
	if params.liveTranslateLanguage != "" {
		if err := ossettings.ToggleLiveTranslate(cr, tconn, true, params.liveTranslateLanguage)(ctx); err != nil {
			s.Fatal("Failed to toggle on live translate: ", err)
		}
	}

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	// No need for loud volume for this test. Turn it down for more comfortable local debugging.
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Log("Non-fatal error: Failed to create new a volume helper (are you running on a VM?): ", err)
	} else {
		err = vh.SetVolume(ctx, 15)
		if err != nil {
			s.Fatal("Failed to set volume: ", err)
		}
	}

	// Open the test page and play the audio.
	conn, err := cr.NewConn(ctx, server.URL+"/live_caption.html")
	if err != nil {
		s.Fatal("Failed to open test web page: ", err)
	}
	defer conn.Close()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	audioPlayButton := nodewith.Name("play").Role(role.Button)
	audioPauseButton := nodewith.Name("pause").Role(role.Button)
	liveCaptionBubble := nodewith.ClassName("CaptionBubbleFrameView")
	liveCaptionContent := nodewith.Name("Hello").Role(role.StaticText)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := uiauto.Combine("Play the audio",
			ui.WaitUntilExists(audioPlayButton),
			ui.DoDefault(audioPlayButton),
			ui.WaitUntilExists(audioPauseButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to play the audio")
		}

		// Not use uiauto.Combine because we want to distinguish between "timeout" and "content is wrong".
		// Because liveCaptionBubble exists only when live caption emits content, if the expected
		// liveCaptionContent doesn't show, we know there's wrong content.
		if err := ui.WaitUntilExists(liveCaptionBubble)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for live caption bubble to show")
		}

		if err := ui.WaitUntilExists(liveCaptionContent)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for correct content")
		}

		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to verify live caption bubble: ", err)
	}
}
