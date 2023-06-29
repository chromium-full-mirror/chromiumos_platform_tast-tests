// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtube"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtubemusic"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinUnpinMediaPod,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the pin/unpin/re-pin for media control pod",
		Contacts: []string{
			"cros-arc-te@google.com",
			"cienet-development@googlegroups.com",
			"cj.tsai@cienet.com",
		},
		BugComponent: "b:1052117", // ChromeOS > Software > ARC++ > EngProd
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc"},
		// There are two apps to be installed in this case.
		Timeout: 2*time.Minute + 2*apputil.InstallationTimeout,
		Fixture: "arcBootedWithPlayStoreQsRevampEnabled",
	})
}

const (
	ytAppLink       = "https://www.youtube.com/watch?v=JE3-LkMqBfM"
	ytAppVideo      = "Whale Songs and AI, for everyone to explore"
	ytMusicVideo    = "Beat It"
	ytMusicSubtitle = "Michael Jackson • 4:19"
)

// PinUnpinMediaPod checks the pin/unpin/re-pin for media control pod.
func PinUnpinMediaPod(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC
	device := s.FixtValue().(*arc.PreData).UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// This test plays music from two ARC++ apps to verify that there are two music display in media control.
	// The media control will be dismissed once another full-screen app has launched.
	// Therefore, this test can only be conducted under clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// The attempt to play media needed to be separated from launching the app since
	// the app window could be in full-screen state (ash.WindowStateFullscreen) by default,
	// which any full-screen window will cause all other medias to be paused automatically and the media controls will be gone.
	var playMediaActions []uiauto.Action

	var currentMediaName string
	for appName, media := range map[string]*apputil.Media{
		youtubemusic.AppName: apputil.NewMedia(ytMusicVideo, ytMusicSubtitle),
		youtube.AppName:      apputil.NewMedia(ytAppLink, ytAppVideo),
	} {
		var err error
		var app apputil.ARCMediaPlayer
		var appPkgName string
		switch appName {
		case youtube.AppName:
			currentMediaName = media.Subtitle
			appPkgName = youtube.PkgName
			app, err = youtube.NewApp(ctx, kb, tconn, a, device)
		case youtubemusic.AppName:
			currentMediaName = media.Query
			appPkgName = youtubemusic.PkgName
			app, err = youtubemusic.New(ctx, kb, tconn, a, device)
		default:
			s.Fatal("Failed to create media app instance: unexpected media source: ", appName)
		}
		if err != nil {
			s.Fatal("Failed to create media app instance: ", err)
		}

		if err := app.Install(ctx); err != nil {
			s.Fatal("Failed to install: ", err)
		}

		if _, err := app.Launch(ctx); err != nil {
			s.Fatal("Failed to launch: ", err)
		}
		defer app.Close(cleanupCtx, cr, s.HasError, filepath.Join(s.OutDir(), appName))

		// The media control will be dismissed once another full-screen app has launched.
		// Therefore, set window state to normal state is essential.
		if _, err := ash.SetARCAppWindowStateAndWait(ctx, tconn, appPkgName, ash.WindowStateNormal); err != nil {
			s.Fatalf("Failed to set %s window state to normal: %v", appPkgName, err)
		}

		// Dismiss mobile prompt before another app launched. Otherwise, there will be two identical UI nodes might fail to be dismissed.
		if err := apputil.DismissMobilePrompt(ctx, tconn); err != nil {
			s.Fatal("Failed to dismiss mobile prompt: ", err)
		}

		// Attempt to play the media after all apps are launched and set as normal state (ash.WindowStateNormal).
		playMediaActions = append(playMediaActions, focusOnAppWindowAndPlay(tconn, appPkgName, app, media))
	}

	for _, action := range playMediaActions {
		if err := action(ctx); err != nil {
			s.Fatal("Failed to play media: ", err)
		}
	}

	ui := uiauto.New(tconn)

	// Unpin media pod if it is pinned by default.
	if err := uiauto.IfSuccessThen(
		ui.WaitUntilExists(quicksettings.PinnedMediaControls),
		quicksettings.UnpinMediaControlsPod(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to ensure media controls pod is unpinned: ", err)
	}

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show quick settings: ", err)
	}
	defer quicksettings.Hide(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_quicksettings")

	// currentMediaName is used to enter the media control pod detail view,
	// where the pin button and all media control panels are located.
	if err := pinAndVerify(ctx, ui, tconn, currentMediaName)(ctx); err != nil {
		s.Fatal("Failed to pin and verify: ", err)
	}

	if err := unpinAndVerify(ctx, ui, tconn)(ctx); err != nil {
		s.Fatal("Failed to unpin and verify: ", err)
	}

	if err := pinAndVerify(ctx, ui, tconn, currentMediaName)(ctx); err != nil {
		s.Fatal("Failed to pin again and verify: ", err)
	}
}

// focusOnAppWindowAndPlay returns an action that plays the media after focusing on the app.
func focusOnAppWindowAndPlay(tconn *chrome.TestConn, pkgName string, player apputil.ARCMediaPlayer, media *apputil.Media) uiauto.Action {
	return func(ctx context.Context) error {
		window, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
		if err != nil {
			return errors.Wrapf(err, "failed to get window %s", pkgName)
		}

		if err := window.ActivateWindow(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to activate window")
		}

		if err := player.Play(ctx, media); err != nil {
			return errors.Wrapf(err, "failed to play app %s", pkgName)
		}

		return nil
	}
}

// unpinAndVerify unpins media pod and verify it is appeared in quick settings.
func unpinAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn) uiauto.Action {
	dialogView := nodewith.Ancestor(quicksettings.MediaControlsDialog)

	mediaControlsPod, err := quicksettings.MediaControlsPod(ctx, tconn)
	if err != nil {
		return func(context.Context) error {
			return errors.Wrap(err, "failed to get media controls pod finder")
		}
	}

	return uiauto.Combine("unpin and find media pod in quick settings",
		ui.LeftClick(quicksettings.PinnedMediaControls),
		ui.WaitUntilExists(dialogView.Role(role.ListItem).NameStartingWith(ytMusicVideo)),
		ui.WaitUntilExists(dialogView.Role(role.ListItem).NameStartingWith(ytAppVideo)),
		quicksettings.UnpinMediaControlsPod(tconn),
		reopenQuickSettings(tconn),
		ui.WaitUntilExists(mediaControlsPod),
	)
}

// pinAndVerify verifies both media control panels are inside detail view,
// then pins media pod and verifies it is disappeared in quick settings.
func pinAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, title string) uiauto.Action {
	detailView := nodewith.Ancestor(quicksettings.MediaControlsDetailView)

	mediaControlsPod, err := quicksettings.MediaControlsPod(ctx, tconn)
	if err != nil {
		return func(context.Context) error {
			return errors.Wrap(err, "failed to get media controls pod finder")
		}
	}

	return uiauto.Combine("pin media pod and verify it is disappeared in quick settings",
		quicksettings.NavigateToMediaControlsSubpage(tconn, title),
		ui.WaitUntilExists(detailView.Role(role.ListItem).NameStartingWith(ytMusicVideo)),
		ui.WaitUntilExists(detailView.Role(role.ListItem).NameStartingWith(ytAppVideo)),
		quicksettings.PinMediaControlsPod(tconn),
		reopenQuickSettings(tconn),
		ui.WaitUntilGone(mediaControlsPod),
	)
}

// reopenQuickSettings reopens quick settings.
func reopenQuickSettings(tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		if err := quicksettings.Hide(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to hide quicksettings")
		}
		return quicksettings.Show(ctx, tconn)
	}
}
