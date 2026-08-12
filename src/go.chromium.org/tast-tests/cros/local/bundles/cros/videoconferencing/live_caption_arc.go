// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakearc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LiveCaptionARC,
		Desc: "Checks on-device live caption works in ARC++",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
			"group:release-health", "release-health_vc",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		Data:         []string{fakearc.AppNameArc},
		Fixture:      a11y.LoggedInARCWithInternalCameraAndEffectsDisabledWithSoda,
		SoftwareDeps: []string{"chrome"},
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

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := ossettings.ToggleLiveCaption(cr, tconn, true)(ctx); err != nil {
		s.Fatal("Failed to toggle on live caption: ", err)
	}
	defer func(ctx context.Context) {
		if err := ossettings.ToggleLiveCaption(cr, tconn, false)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to toggle off live caption in cleanup: ", err)
		}
	}(cleanupCtx)

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(ctx)

	fakearc.Install(ctx, s)

	arcApp := fakearc.Launch(ctx, s, tconn, cr, kb)

	defer arcApp.Close(cleanupCtx)

	// Play audio and wait for the caption.
	if err := common.PlayAudioAndVerifyBuble(ctx, tconn, arcApp.PlayAudio); err != nil {
		s.Fatal("Failed to validate live caption: ", err)
	}
}
