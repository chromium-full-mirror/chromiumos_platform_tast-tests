// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/wmp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CaptureModeScreenshot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Take a fullscreen, partial or window screen shot by pressing the 'Screen capture' button in the quick settings and verify the existence of the screenshot",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"michelefan@chromium.org",
		},
		// ChromeOS > Software > ScreenCapture
		BugComponent: "b:1253115",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{
			{
				Name: "fullscreen",
				Val:  wmp.FullScreen,
			},
			{
				Name: "partialscreen",
				Val:  wmp.PartialScreen,
			},
			{
				Name:      "window",
				Val:       wmp.Window,
				ExtraAttr: []string{"group:hw_agnostic"},
			},
		},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-936ea36a-b93f-4127-9260-9975e69365fa",
		}},
	})
}

func CaptureModeScreenshot(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	if err := wmp.DeleteAllScreenCaptureFiles(downloadsPath, true /*deleteScreenRecording=*/, true /*deleteScreenshots=*/); err != nil {
		s.Fatal("Failed to delete all screenshots: ", err)
	}

	// Reserve 15 seconds for the cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	if err := wmp.LaunchScreenCapture(ctx, tconn); err != nil {
		s.Fatal("Failed to launch 'Screen capture': ", err)
	}
	defer wmputils.EnsureCaptureModeActivated(tconn, false)(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	s.Log(ctx, "Launch 'Screen capture' and capture screenshot")
	source := s.Param().(wmp.CaptureModeSource)

	if err := wmp.CaptureScreenshot(ctx, tconn, source)(ctx); err != nil {
		s.Fatalf("Failed to capture %v screenshot: %v", source, err)
	}
	defer func() {
		if err := wmp.DeleteAllScreenCaptureFiles(downloadsPath, true /*deleteScreenRecording=*/, true /*deleteScreenshots=*/); err != nil {
			s.Log(ctx, "Failed to delete the screenshot")
		}
	}()

	s.Log(ctx, "Check the existence and the size of the screenshot")
	if err := wmp.CheckScreenshot(ctx, tconn, downloadsPath, source); err != nil {
		s.Fatal("Failed to verify the screenshot: ", err)
	}
}
