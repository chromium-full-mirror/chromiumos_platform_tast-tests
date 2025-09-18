// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/vlc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtubemusic"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PinUnpinMediaPod,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Check the pin/unpin/re-pin for media control pod",
		Contacts: []string{
			"cros-arc-te@google.com", "mattlui@google.com",
		},
		BugComponent: "b:1052117", // ChromeOS > Software > ARC++ > EngProd
		// Disable the this test since this no Google team has taken ownership. See b/403396055.
		// Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "gaia"},
		Data:         []string{vlcVideo},
		// There are two apps to be installed in this case.
		Timeout: 2*time.Minute + 2*apputil.InstallationTimeout,
		Fixture: "arcBootedWithPlayStore",
	})
}

const (
	vlcVideo         = "cars_144_h264.mp4"
	vlcVideoSubtitle = "cars_144"
	ytMusicVideo     = "Beat It"
	ytMusicSubtitle  = "Michael Jackson • 4:19"
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

	res := &arcAppTestResources{
		chrome: cr,
		kb:     kb,
		tconn:  tconn,
		arc:    a,
		device: device,
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}

	// Copy test files to Downloads directory.
	fileLocation := filepath.Join(downloadsPath, vlcVideo)
	if _, err := os.Stat(fileLocation); os.IsNotExist(err) {
		if err := fsutil.CopyFile(s.DataPath(vlcVideo), fileLocation); err != nil {
			s.Fatal("Failed to copy file: ", err)
		}
		defer os.Remove(fileLocation)
	} else if err != nil {
		s.Fatal("Failed to get test file info: ", err)
	}

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

	for appName, media := range map[string]*apputil.Media{
		youtubemusic.AppName: apputil.NewMedia(ytMusicVideo, ytMusicSubtitle),
		vlc.AppName:          apputil.NewMedia(vlcVideo, vlcVideoSubtitle),
	} {
		var err error
		var app apputil.ARCMediaPlayer
		var appPkgName string
		switch appName {
		case vlc.AppName:
			appPkgName = vlc.PackageName
			app = newVLCVideoPlayer(ctx, res)
		case youtubemusic.AppName:
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
		quicksettings.UnpinMediaControlsPod(tconn, kb),
	)(ctx); err != nil {
		s.Fatal("Failed to ensure media controls pod is unpinned: ", err)
	}

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show quick settings: ", err)
	}
	defer quicksettings.Hide(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_quicksettings")

	if err := pinAndVerify(ctx, ui, tconn)(ctx); err != nil {
		s.Fatal("Failed to pin and verify: ", err)
	}

	if err := unpinAndVerify(ctx, ui, tconn, kb)(ctx); err != nil {
		s.Fatal("Failed to unpin and verify: ", err)
	}

	if err := pinAndVerify(ctx, ui, tconn)(ctx); err != nil {
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
func unpinAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) uiauto.Action {
	dialogView := nodewith.Ancestor(quicksettings.MediaControlsDialog)

	return uiauto.Combine("unpin and find media pod in quick settings",
		ui.LeftClick(quicksettings.PinnedMediaControls),
		ui.WaitUntilExists(dialogView.Role(role.StaticText).NameStartingWith(ytMusicVideo)),
		ui.WaitUntilExists(dialogView.Role(role.StaticText).NameStartingWith(vlcVideoSubtitle)),
		quicksettings.UnpinMediaControlsPod(tconn, kb),
		reopenQuickSettings(tconn),
		ui.WaitUntilExists(quicksettings.MediaControlsPod()),
	)
}

// pinAndVerify verifies both media control panels are inside detail view,
// then pins media pod and verifies it is disappeared in quick settings.
func pinAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn) uiauto.Action {
	detailView := nodewith.Ancestor(quicksettings.MediaControlsDetailView)

	return uiauto.Combine("pin media pod and verify it is disappeared in quick settings",
		quicksettings.NavigateToMediaControlsSubpage(tconn),
		ui.WaitUntilExists(detailView.Role(role.StaticText).NameStartingWith(ytMusicVideo)),
		ui.WaitUntilExists(detailView.Role(role.StaticText).NameStartingWith(vlcVideoSubtitle)),
		quicksettings.PinMediaControlsPod(tconn),
		reopenQuickSettings(tconn),
		ui.EnsureGoneFor(quicksettings.MediaControlsPod(), 10*time.Second),
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

// vlcVideoPlayer represents the media app: VLC.
type vlcVideoPlayer struct {
	*vlc.Vlc
	res *arcAppTestResources
}

// arcAppTestResources represents the resources needed for ARC app test.
type arcAppTestResources struct {
	chrome *chrome.Chrome
	kb     *input.KeyboardEventWriter
	tconn  *chrome.TestConn
	arc    *arc.ARC
	device *ui.Device
}

// vlcVideoPlayer is built to conform to ARCMediaPlayer interface.
var _ apputil.ARCMediaPlayer = (*vlcVideoPlayer)(nil)

// newVLCVideoPlayer returns vlcVideoPlayer instance.
func newVLCVideoPlayer(ctx context.Context, resources *arcAppTestResources) *vlcVideoPlayer {
	return &vlcVideoPlayer{
		res: resources,
	}
}

// Play searches the specified media source and plays it by vlcVideoPlayer.
func (vp *vlcVideoPlayer) Play(ctx context.Context, media *apputil.Media) error {
	if err := vp.EnterDownloadFolder(ctx); err != nil {
		return err
	}
	return vp.Vlc.Play(ctx, &vlc.MediaInfo{
		FileName: media.Query,
		FileType: vlc.Video,
	})
}

// Install installs the vlcVideoPlayer.
func (vp *vlcVideoPlayer) Install(ctx context.Context) error {
	var err error
	if vp.Vlc, err = vlc.NewVLCPlayer(ctx, vp.res.chrome, vp.res.kb, vp.res.tconn, vp.res.arc, vp.res.device); err != nil {
		return err
	}
	return nil
}

// Launch launches the vlcVideoPlayer.
func (vp *vlcVideoPlayer) Launch(ctx context.Context) (time.Duration, error) {
	return 0, vp.Vlc.Launch(ctx)
}
