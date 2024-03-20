// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arcvideoplayback

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ExoPlayerAPKFileName is the apk file name of ExoPlayer App.
	ExoPlayerAPKFileName = "arc_video_playback/exoplayer.apk"
	exoPlayerPkg         = "androidx.media3.demo.main"
	exoPlayerIDPrefix    = exoPlayerPkg + ":id/"
)

// ExoPlayerApp defines the members related to ExoPlayer.
type ExoPlayerApp struct {
	tconn    *chrome.TestConn
	kb       *input.KeyboardEventWriter
	a        *arc.ARC
	d        *androidui.Device
	cr       *chrome.Chrome
	dataPath func(string) string
}

// NewExoPlayerApp creates an instance of ExoPlayer app.
func NewExoPlayerApp(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *androidui.Device, dataPath func(string) string) VideoApp {
	return &ExoPlayerApp{
		tconn:    tconn,
		kb:       kb,
		a:        a,
		d:        d,
		cr:       cr,
		dataPath: dataPath,
	}
}

// Install the ExoPlayer app.
func (e *ExoPlayerApp) Install(ctx context.Context) error {
	return util.InstallAppFromAPKPath(ctx, e.a, e.d, exoPlayerPkg, e.dataPath(ExoPlayerAPKFileName))
}

// Uninstall the ExoPlayer app if it has been installed.
func (e *ExoPlayerApp) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, e.a, exoPlayerPkg)
}

// GetAppVersion returns the version of the ExoPlayer app.
func (e *ExoPlayerApp) GetAppVersion(ctx context.Context) (string, error) {
	return util.GetAppVersion(ctx, e.a, exoPlayerPkg)
}

// CopyFileToFolder copies the video file to Android temp directory and check if
// the file has finished copying. Remember to call the cleanup function to delete
// the file created in this function.
func (e *ExoPlayerApp) CopyFileToFolder(ctx context.Context, videoPath string) (cleanup func() error, retErr error) {
	targetFilePath, err := e.a.PushFileToTmpDir(ctx, e.dataPath(videoPath))
	if err != nil {
		return nil, errors.Wrap(err, "failed to push video file to ARC")
	}
	defer func() {
		if retErr != nil {
			e.a.Command(ctx, "rm", targetFilePath).Run()
		}
	}()

	fileName := filepath.Base(videoPath)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		files, err := e.a.Command(ctx, "ls", path.Join(adb.AndroidTmpDirPath, fileName)).Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to glob video file")
		}
		if len(files) == 0 {
			return errors.New("file not found")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to find video file in Android temp directory")
	}
	return func() error { return e.a.Command(ctx, "rm", targetFilePath).Run() }, nil
}

// Launch the ExoPlayer app.
func (e *ExoPlayerApp) Launch(ctx context.Context) error {
	return util.LaunchApp(ctx, e.tconn, e.kb, apps.ExoPlayer)
}

// PlayVideoInFullScreen plays the video in full screen.
func (e *ExoPlayerApp) PlayVideoInFullScreen(videoFileName string) uiauto.Action {
	return uiauto.Combine("play the video in full screen",
		e.PlayVideoInLoop(videoFileName),
		e.DismissPrompts,
		e.EnsurePlaying,
		e.EnterFullScreen,
	)
}

// PlayVideoInLoop plays the video in loop.
func (e *ExoPlayerApp) PlayVideoInLoop(videoFileName string) uiauto.Action {
	return func(ctx context.Context) error {
		args := []string{"start", "-a", "androidx.media3.demo.main.action.VIEW", "-d"}
		videoPath := fmt.Sprintf("file:///%s", path.Join(adb.AndroidTmpDirPath, videoFileName))
		args = append(args, videoPath, "--es", "repeat_mode", "ALL")

		if _, err := e.a.Command(ctx, "am", args...).Output(testexec.DumpLogOnError); err != nil {
			return err
		}
		return nil
	}
}

// DismissPrompts dismisses prompts and grants storage permission.
func (e *ExoPlayerApp) DismissPrompts(ctx context.Context) error {
	storageAllowBtn := e.d.Object(androidui.ResourceIDMatches(".*permission_allow_button$"), androidui.TextMatches("(?i)Allow"))
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cuj.WaitForExists(storageAllowBtn, defaultUITimeout)(ctx); err != nil {
			return nil
		}
		if err := storageAllowBtn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click allow button")
		}
		return errors.New("not all prompts have been cleared")
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to dismiss prompts")
	}
	return nil
}

// EnterFullScreen switches ExoPlayer video to full screen.
func (e *ExoPlayerApp) EnterFullScreen(ctx context.Context) error {
	if w, err := ash.GetARCAppWindowInfo(ctx, e.tconn, exoPlayerPkg); err != nil {
		return errors.Wrap(err, "failed to get ARC window info")
	} else if w.State == ash.WindowStateMaximized {
		return nil
	}

	if err := e.ChangeToResizable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizable")
	}

	if _, err := ash.SetARCAppWindowStateAndWait(ctx, e.tconn, exoPlayerPkg, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to set window state to full screen")
	}
	return nil
}

// ExitFullScreen exits ExoPlayer video from full screen.
func (e *ExoPlayerApp) ExitFullScreen(ctx context.Context) error {
	if w, err := ash.GetARCAppWindowInfo(ctx, e.tconn, exoPlayerPkg); err != nil {
		return errors.Wrap(err, "failed to get ARC window info")
	} else if w.State == ash.WindowStateNormal {
		return nil
	}

	if err := e.ChangeToResizable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizable")
	}

	if _, err := ash.SetARCAppWindowStateAndWait(ctx, e.tconn, exoPlayerPkg, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to set window state to normal")
	}
	return nil
}

// EnsurePlaying ensures the video is playing now.
func (e *ExoPlayerApp) EnsurePlaying(ctx context.Context) error {
	errorMessage := e.d.Object(androidui.ID(exoPlayerIDPrefix + "exo_error_message"))
	if err := cuj.WaitForExists(errorMessage, defaultUITimeout)(ctx); err == nil {
		errorMsg, err := errorMessage.GetText(ctx)
		if err == nil {
			return errors.Errorf("error message appears: %s", errorMsg)
		}
		return errors.New("error message appears")
	}
	ui := uiauto.New(e.tconn)
	playerView := e.d.Object(androidui.ID(exoPlayerIDPrefix + "player_view"))
	playBtn := e.d.Object(androidui.ID(exoPlayerIDPrefix+"exo_play_pause"), androidui.ClassName(imageBtnClassName))
	playVideo := uiauto.NamedCombine("play video",
		cuj.FindAndClick(playBtn, defaultUITimeout),
		cuj.WaitUntilGone(playBtn, defaultUITimeout),
	)
	return uiauto.Combine("ensure video is playing",
		cuj.WaitForExists(playerView, defaultUITimeout),
		ui.Retry(retryTimes, uiauto.IfFailThen(cuj.WaitUntilGone(playBtn, defaultUITimeout), playVideo)),
	)(ctx)
}

// ChangeToResizable changes the ExoPlayer window to resizable.
func (e *ExoPlayerApp) ChangeToResizable(ctx context.Context) error {
	ui := uiauto.New(e.tconn)

	exoPlayerWindow := nodewith.Role(role.Window).Name(apps.ExoPlayer.Name).First()
	centerBtn := nodewith.Role(role.Button).HasClass("FrameCenterButton").Ancestor(exoPlayerWindow)
	resizeBtn := nodewith.Role(role.Button).HasClass("FrameSizeButton").NameRegex(regexp.MustCompile("(Maximize|Restore)")).Ancestor(exoPlayerWindow)
	foundBtn, err := ui.FindAnyExists(ctx, centerBtn, resizeBtn)
	if err != nil {
		if strings.Contains(err.Error(), nodewith.ErrNotFound) {
			return errors.Wrap(err, "no 'Frame Center' or resize button to change window size")
		}
		return errors.Wrap(err, "failed to check the existence of resize button")
	}
	if foundBtn == resizeBtn {
		return nil
	}

	const resizable = "Resizable"
	centerBtnInfo, err := ui.Info(ctx, centerBtn)
	if err != nil {
		return errors.Wrap(err, "failed to get center button info")
	}
	if centerBtnInfo.Name == resizable {
		return nil
	}

	resizableBtn := nodewith.Role(role.MenuItem).Name(resizable)
	allowWin := nodewith.Role(role.Dialog).NameStartingWith("Allow resizing").HasClass("RootView")
	allowBtn := nodewith.Role(role.Button).Name("Allow").Ancestor(allowWin)
	return uiauto.NamedCombine("change ARC window to be resizable",
		ui.LeftClickUntil(centerBtn, ui.WithTimeout(defaultUITimeout).WaitUntilExists(resizableBtn)),
		ui.LeftClick(resizableBtn),
		ui.WithTimeout(defaultUITimeout).WaitUntilExists(allowWin),
		ui.LeftClick(allowBtn),
	)(ctx)
}

// Close the resources related to video.
func (e *ExoPlayerApp) Close(ctx context.Context) error {
	return util.CloseApp(ctx, e.tconn, exoPlayerPkg)
}
