// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/wmp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowRecorder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the window recorder Tast API works",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > ScreenCapture
		BugComponent: "b:1253115",
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-936ea36a-b93f-4127-9260-9975e69365fa",
		}},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      1 * time.Minute,
	})
}

// WindowRecorder tests the window recorder using Screen Share and Screen Capture.
func WindowRecorder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's download path: ", err)
	}

	// chrome.New() should empty the downloads folder but we want to be certain.
	if err := wmp.DeleteAllRecordings(downloadsPath); err != nil {
		s.Fatal("Failed to delete recordings: ", err)
	}

	if _, err := filesapp.Launch(ctx, tconn); err != nil {
		s.Fatal("Failed to launch the files app: ", err)
	}

	if err := wmp.LaunchScreenCapture(ctx, tconn); err != nil {
		s.Fatal("Failed screen capture test: ", err)
	}
	if err := wmp.RecordWindowScreenCapture(ctx, tconn, downloadsPath); err != nil {
		s.Fatal("Failed to record a window using screen capture: ", err)
	}

	if err := wmp.RecordWindowScreenShare(ctx, tconn, downloadsPath); err != nil {
		s.Fatal("Failed screen share test: ", err)
	}

	if err := wmp.DeleteAllRecordings(downloadsPath); err != nil {
		s.Fatal("Failed to delete recordings: ", err)
	}
}
