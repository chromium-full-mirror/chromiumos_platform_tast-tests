// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package media

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ControlMedia,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Controls the media bubble",
		Contacts: []string{
			"cros-status-area@google.com",
			"jiamingc@chromium.org",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:hw_agnostic"},
	})
}

// ControlMedia verifies that we can open the media tray, change song and start/stop song.
func ControlMedia(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// This is a playlist from the Google official account on Youtube.
	conn, err := cr.NewConn(ctx,
		"https://www.youtube.com/watch?v=lj0bFX9HXeE&list=PL590L5WQmH8dUdwu3ZWuy5xh8j2iGgApr")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	ui := uiauto.New(tconn)

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get windows: ", err)
	}

	// Verify that there is one and only one window.
	if wsCount := len(ws); wsCount != 1 {
		s.Fatal("Expected 1 window; found ", wsCount)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, ws[0].ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize window: ", err)
	}

	// Clicking on top left of the screen should start playing since the window is maximized.
	topLeftPt := coords.NewPoint(200, 200)
	if err := mouse.Click(tconn, topLeftPt, mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to click a point on the screen: ", err)
	}

	// Showing media tray.
	mediaTray := nodewith.NameContaining("Control your music,").HasClass("ImageView")
	if err := ui.WaitUntilExists(mediaTray)(ctx); err != nil {
		s.Fatal("Failed to find media tray after playing a video: ", err)
	}

	// Open media view.
	if err := ui.LeftClick(mediaTray)(ctx); err != nil {
		s.Fatal("Failed to click the media tray: ", err)
	}

	mediaItemList := nodewith.HasClass("MediaItemUIListView")
	if err := ui.WaitUntilExists(mediaItemList)(ctx); err != nil {
		s.Fatal("Failed to find media item list after clicking on the media tray: ", err)
	}

	pauseButton := nodewith.Name("Pause").HasClass("ToggleImageButton")
	if err := ui.WaitUntilExists(pauseButton)(ctx); err != nil {
		s.Fatal("Failed to find the pause button after clicking on the media tray: ", err)
	}

	// Pause the playing.
	if err := ui.DoDefault(pauseButton)(ctx); err != nil {
		s.Fatal("Failed to click the pause button: ", err)
	}

	playButton := nodewith.Name("Play").HasClass("ToggleImageButton")
	if err := ui.WaitUntilExists(playButton)(ctx); err != nil {
		s.Fatal("Failed to find the play button after clicking on the pause button: ", err)
	}

	// Continue playing.
	if err := ui.DoDefault(playButton)(ctx); err != nil {
		s.Fatal("Failed to click the play button: ", err)
	}

	if err := ui.WaitUntilExists(pauseButton)(ctx); err != nil {
		s.Fatal("Failed to find the pause button after clicking on the play button: ", err)
	}

	// (TODO: b/237826754) Need to test the previous and next button after this bug is fixed.
	// nextButton := nodewith.Name("Next Track").HasClass("ImageButton")
	// if err := ui.WaitUntilExists(nextButton)(ctx); err != nil {
	// 	s.Fatal("Failed to find the next button", err)
	// }
}
