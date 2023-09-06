// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc/apputil/vlc"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LiveCaptionARC,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks on-device live caption works in ARC++",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:external-dependency",
			"group:video_conference",
			"video_conference_per_build",
		},
		Data:         []string{data.SpeechInputFile},
		Fixture:      fixture.GAIALoggedInARCWithInternalCameraAndEffectsEnabled,
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		SearchFlags: []*testing.StringPair{
			{
				// Live captions display on ARC++ App.
				Key:   "feature_id",
				Value: "screenplay-1ff9a1a0-5ccc-4085-a74e-ed00855530d9",
			},
		},
	})
}

func LiveCaptionARC(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(fixture.FixtData).ARC()
	d := s.FixtValue().(fixture.FixtData).UIDevice()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Prepare test audio file.
	audioFile := data.SpeechInputFile
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	testFileLocation := filepath.Join(downloadsPath, audioFile)
	playDuration := 30 * time.Second
	if err := wav.RepeatForDuration(ctx, s.DataPath(data.SpeechInputFile), testFileLocation, playDuration); err != nil {
		s.Fatal("Cannot prepare wav file: ", err)
	}
	defer os.Remove(testFileLocation)

	if err := ossettings.ToggleLiveCaption(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to toggle on live caption: ", err)
	}
	defer func(ctx context.Context) {
		if err := ossettings.ToggleLiveCaption(cr, tconn, false)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to toggle off live caption in cleanup: ", err)
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	vlcPlayer, err := vlc.NewVLCPlayer(ctx, cr, kb, tconn, a, d)
	if err != nil {
		s.Fatal("Failed to create VLC instance: ", err)
	}
	defer vlcPlayer.Close(cleanupCtx, cr, s.HasError, s.OutDir())

	if err := vlcPlayer.Launch(ctx); err != nil {
		s.Fatalf("Failed to launch app %q: %v", vlc.AppName, err)
	}

	testing.ContextLog(ctx, "Enter audio folder")
	if err := vlcPlayer.EnterAudioFolder(ctx); err != nil {
		testing.ContextLog(ctx, "Not entering audio folder or already in the folder")
	}

	if err := vlcPlayer.PlayAudio(ctx, audioFile); err != nil {
		s.Fatalf("Failed to play audio file %q: %v", audioFile, err)
	}

	ui := uiauto.New(tconn)
	liveCaptionBubble := nodewith.ClassName("CaptionBubbleFrameView")
	liveCaptionContent := nodewith.NameContaining("Hello").Role(role.StaticText)
	if err := uiauto.NamedCombine("wait for live caption bubble",
		ui.WaitUntilExists(liveCaptionBubble),
		ui.WaitUntilExists(liveCaptionContent),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for live caption bubble: ", err)
	}
}
