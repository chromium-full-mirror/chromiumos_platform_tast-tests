// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package youtube

import (
	"context"
	"strings"
	"time"

	androidui "chromiumos/tast/common/android/ui"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	vlcPackage         = "org.videolan.vlc"
	vlcPlayPauseBtnID  = vlcPackage + ":id/player_overlay_play"
	vlcPlayBtnDesc     = "Play"
	vlcPauseBtnDesc    = "Pause"
	vlcPlayerViewID    = vlcPackage + ":id/player_ui_container"
	vlcBottomSheetID   = vlcPackage + ":id/design_bottom_sheet"
	vlcOptionsDialogID = vlcPackage + ":id/bottom_sheet_list_view"
	waitTime           = 3 * time.Second
)

// VlcApp defines the members related to VLC media player.
type VlcApp struct {
	tconn  *chrome.TestConn
	kb     *input.KeyboardEventWriter
	a      *arc.ARC
	d      *androidui.Device
	act    *arc.Activity
	outDir string
}

// NewVlcApp creates an instance of VLC app.
func NewVlcApp(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *androidui.Device, outDir string) *VlcApp {
	return &VlcApp{
		tconn:  tconn,
		kb:     kb,
		a:      a,
		d:      d,
		outDir: outDir,
	}
}

// Install installs the VLC app.
func (y *VlcApp) Install(ctx context.Context) error {
	// TODO(b/267957319): we need to support installing app from apk to get rid of playstore.
	if err := playstore.InstallOrUpdateAppAndClose(ctx, y.tconn, y.a, y.d, vlcPackage, &playstore.Options{TryLimit: -1}); err != nil {
		return errors.Wrap(err, "failed to install VLC app from playstore")
	}
	appVersion, _, err := dumpAppInfo(ctx, y.a, y.d, vlcPackage)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s version: %v", vlcPackage, appVersion)
	}
	return nil
}

// OpenAndPlayVideo opens a video on vlc app.
func (y *VlcApp) OpenAndPlayVideo(video VideoSrc) uiauto.Action {
	return func(ctx context.Context) (err error) {
		testing.ContextLog(ctx, "Open VLC app")

		const (
			vlcApp         = "VLC App"
			vlcAct         = "org.videolan.vlc.StartActivity"
			skipButtonID   = vlcPackage + ":id/skip_button"
			navMoreID      = vlcPackage + ":id/nav_more"
			newStreamID    = vlcPackage + ":id/mrl_item_title"
			openEditTextID = vlcPackage + ":id/mrl_edit"
			closeTipsID    = vlcPackage + ":id/close"
		)

		if appStartTime, y.act, err = cuj.OpenAppAndGetStartTime(ctx, y.tconn, y.a, vlcPackage, vlcApp, vlcAct); err != nil {
			return errors.Wrap(err, "failed to get app start time")
		}

		if err := uiauto.Combine("close notification",
			y.kb.TypeAction("abc"),
			y.kb.AccelAction("enter"),
		)(ctx); err != nil {
			return err
		}

		skip := y.d.Object(androidui.ID(skipButtonID))
		if err := cuj.ClickIfExist(skip, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Skip'")
		}

		navMore := y.d.Object(androidui.ID(navMoreID))
		if err := cuj.FindAndClick(navMore, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'navMore'")
		}

		newStream := y.d.Object(androidui.ID(newStreamID))
		if err := cuj.FindAndClick(newStream, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'newStream'")
		}

		openEditText := y.d.Object(androidui.ID(openEditTextID))
		if err := cuj.FindAndClick(openEditText, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'openEditText'")
		}

		if err := uiauto.Combine("type video url",
			y.kb.TypeAction(video.URL),
			y.kb.AccelAction("enter"),
		)(ctx); err != nil {
			return err
		}

		closeTips := y.d.Object(androidui.ID(closeTipsID))
		if err := cuj.ClickIfExist(closeTips, 5*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'closeTips'")
		}

		return nil
	}
}

// EnterFullscreen switches vlc video to fullscreen.
func (y *VlcApp) EnterFullscreen(ctx context.Context) error {
	startTime := time.Now()

	if currentState, err := y.act.GetWindowState(ctx); err == nil && currentState == arc.WindowStateFullscreen {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to get window state")
	}

	if err := y.changeToResizeable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizeable")
	}

	if err := y.act.SetWindowState(ctx, y.tconn, arc.WindowStateFullscreen); err != nil {
		return errors.Wrap(err, "failed to set window state to full screen")
	}

	if err := ash.WaitForARCAppWindowState(ctx, y.tconn, vlcPackage, ash.WindowStateFullscreen); err != nil {
		return errors.Wrap(err, "failed to wait window state change")
	}

	testing.ContextLogf(ctx, "Elapsed time when doing enter fullscreen %.3f s", time.Since(startTime).Seconds())
	return nil
}

// ExitFullScreen exists vlc video from fullscreen.
func (y *VlcApp) ExitFullScreen(ctx context.Context) error {
	startTime := time.Now()

	if currentState, err := y.act.GetWindowState(ctx); err == nil && currentState == arc.WindowStateNormal {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to get window state")
	}

	if err := y.changeToResizeable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizeable")
	}

	if err := y.act.SetWindowState(ctx, y.tconn, arc.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to set window state to normal")
	}

	if err := ash.WaitForARCAppWindowState(ctx, y.tconn, vlcPackage, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to wait window state change")
	}

	testing.ContextLogf(ctx, "Elapsed time when doing exist fullscreen %.3f s", time.Since(startTime).Seconds())

	return nil
}

// PauseAndPlayVideo verifies video playback on vlc app.
func (y *VlcApp) PauseAndPlayVideo(ctx context.Context) error {
	testing.ContextLog(ctx, "Pause and play video")

	playerView := y.d.Object(androidui.ID(vlcPlayerViewID))
	pauseBtn := y.d.Object(androidui.ID(vlcPlayPauseBtnID), androidui.Description(vlcPauseBtnDesc))
	playBtn := y.d.Object(androidui.ID(vlcPlayPauseBtnID), androidui.Description(vlcPlayBtnDesc))

	startTime := time.Now()
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := cuj.FindAndClick(playerView, waitTime)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find/click the player view in %s", waitTime)
		}

		if err := cuj.FindAndClick(pauseBtn, waitTime)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find/click the pause button in %s", waitTime)
		}

		if err := playBtn.WaitForExists(ctx, waitTime); err != nil {
			return errors.Wrap(err, "failed to find the play button in 2s")
		}

		// Immediately clicking the target button sometimes doesn't work.
		if err := testing.Sleep(ctx, waitTime); err != nil {
			return errors.Wrap(err, "failed to sleep before clicking play button")
		}
		if err := playBtn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click the play button")
		}
		if err := pauseBtn.WaitForExists(ctx, waitTime); err != nil {
			return errors.Wrapf(err, "failed to find the pause button in %s", waitTime)
		}

		// Keep the video playing for a short time.
		if err := testing.Sleep(ctx, waitTime); err != nil {
			return errors.Wrap(err, "failed to sleep while video is playing")
		}

		testing.ContextLogf(ctx, "Elapsed time when checking the playback status of vlc app: %.3f s", time.Since(startTime).Seconds())
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 2 * time.Minute})
}

// IsPlaying checks if the video is playing now.
func (y *VlcApp) IsPlaying() uiauto.Action {
	return uiauto.NamedAction("verify video is playing", func(ctx context.Context) error {
		playerView := y.d.Object(androidui.ID(vlcPlayerViewID))
		playBtn := y.d.Object(androidui.ID(vlcPlayPauseBtnID), androidui.Description(vlcPlayBtnDesc))
		return y.ensureVideoPlaying(ctx, playerView, playBtn)
	})
}

// Close the resources related to video.
func (y *VlcApp) Close(ctx context.Context) {
	if y.act != nil {
		if err := y.act.Stop(ctx, y.tconn); err != nil {
			testing.ContextLogf(ctx, "Failed to stop activity: %s", err)
		}
		y.act.Close(ctx)
		y.act = nil
	}
}

func (y *VlcApp) changeToResizeable(ctx context.Context) error {
	ui := uiauto.New(y.tconn)

	centerBtn := nodewith.Role(role.Button).ClassName("FrameCenterButton")
	if err := ui.Exists(centerBtn)(ctx); err != nil {
		if strings.Contains(err.Error(), nodewith.ErrNotFound) {
			// If there is no center button to change window size, we should just return.
			return nil
		}
		return errors.Wrap(err, "failed to check the existence of center button")
	}

	resizable := "Resizable"
	centerBtnInfo, err := ui.Info(ctx, centerBtn)
	if err != nil {
		return errors.Wrap(err, "failed to get center button info")
	}

	if centerBtnInfo.Name == resizable {
		return nil
	}

	testing.ContextLog(ctx, "Change ARC window to be resizable")
	resizableBtn := nodewith.Role(role.MenuItem).Name(resizable).ClassName("Button")
	if err := ui.LeftClickUntil(centerBtn, ui.WithTimeout(waitTime).WaitUntilExists(resizableBtn))(ctx); err != nil {
		return errors.Wrap(err, "failed to click the center button to show menu items")
	}

	if err := ui.LeftClick(resizableBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the Resizable option")
	}

	allowWin := nodewith.Role(role.Dialog).NameStartingWith("Allow resizing").ClassName("RootView")
	allowBtn := nodewith.Role(role.Button).Name("Allow").Ancestor(allowWin)
	if err := ui.WithTimeout(waitTime).WaitUntilExists(allowWin)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the 'Allow resizing' window to exist")
	}

	return ui.LeftClick(allowBtn)(ctx)
}

func (y *VlcApp) ensureVideoPlaying(ctx context.Context, playerView, playBtn *androidui.Object) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := cuj.FindAndClick(playerView, 2*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to find/click the player view in 2s")
		}
		if err := playBtn.WaitForExists(ctx, 2*time.Second); err == nil {
			testing.ContextLog(ctx, "Video is paused; resuming video")
			return playBtn.Click(ctx)
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second})
}
