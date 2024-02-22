// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcvideoplayback implements helpers for the power.ARCVideoPlayback* tests.
package arcvideoplayback

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	mxPlayerPackage    = "com.mxtech.videoplayer.ad"
	mxPlayerIDPrefix   = mxPlayerPackage + ":id/"
	permissionIDPrefix = "com.android.permissioncontroller:id/"

	imageBtnClassName = "android.widget.ImageButton"
	textClassName     = "android.widget.TextView"
	layoutClassName   = "android.widget.LinearLayout"
	buttonClassName   = "android.widget.Button"

	retryTimes       = 3
	defaultUITimeout = 5 * time.Second
	longUITimeout    = 15 * time.Second
)

// MxPlayerApp defines the members related to MX Player.
type MxPlayerApp struct {
	tconn    *chrome.TestConn
	kb       *input.KeyboardEventWriter
	a        *arc.ARC
	d        *androidui.Device
	cr       *chrome.Chrome
	dataPath func(string) string
}

// NewMxPlayerApp creates an instance of MX Player app.
func NewMxPlayerApp(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *androidui.Device, dataPath func(string) string) VideoApp {
	return &MxPlayerApp{
		tconn:    tconn,
		kb:       kb,
		a:        a,
		d:        d,
		cr:       cr,
		dataPath: dataPath,
	}
}

// Install the MX Player app.
func (m *MxPlayerApp) Install(ctx context.Context) error {
	return util.InstallApp(ctx, m.tconn, m.a, m.d, mxPlayerPackage)
}

// Uninstall the MX Player app if it has been installed.
func (m *MxPlayerApp) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, m.a, mxPlayerPackage)
}

// CopyFileToFolder copies the video file to the 'Downloads' folder and check if
// the file has finished copying. Remember to call the cleanup function to delete the file
// created in this function.
func (m *MxPlayerApp) CopyFileToFolder(ctx context.Context, videoPath string) (cleanup func() error, retErr error) {
	downloadsPath, err := cryptohome.DownloadsPath(ctx, m.cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve user's Downloads path")
	}
	fileName := filepath.Base(videoPath)
	targetFilePath := filepath.Join(downloadsPath, fileName)
	if err := fsutil.CopyFile(m.dataPath(videoPath), targetFilePath); err != nil {
		return nil, errors.Wrap(err, "failed to copy the file to the 'Downloads' folder")
	}
	defer func() {
		if retErr != nil {
			os.Remove(targetFilePath)
		}
	}()

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		files, err := filepath.Glob(targetFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to glob video file")
		}
		if len(files) == 0 {
			return errors.New("file not found")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to find video file in Downloads folder")
	}
	return func() error { return os.Remove(targetFilePath) }, nil
}

// Launch the MX Player app.
func (m *MxPlayerApp) Launch(ctx context.Context) error {
	return util.LaunchApp(ctx, m.tconn, m.kb, apps.MxPlayer)
}

// DismissPrompts dismisses prompts and grants storage permission.
func (m *MxPlayerApp) DismissPrompts(ctx context.Context) error {
	otherOptionBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"other_layout"), androidui.ClassName(layoutClassName))
	// There might be two different arc dump hierarchies of granting storage permission that affect how nodes are captured.
	openSettingsBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"storage_permission_accept"), androidui.Text("OPEN SETTINGS"))
	storageAllowBtn := m.d.Object(androidui.ResourceIDMatches(".*permission_allow_button$"), androidui.TextMatches("(?i)Allow"))
	var foundObject *androidui.Object
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// The "This app is designed for mobile" prompt needs to be dismissed to enter the app.
		if err := apputil.DismissMobilePrompt(ctx, m.tconn); err != nil {
			return errors.Wrap(err, `failed to dismiss "This app is designed for mobile" prompt`)
		}
		var err error
		foundObject, err = cuj.FindAnyExists(ctx, defaultUITimeout, otherOptionBtn, openSettingsBtn, storageAllowBtn)
		if err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to find objects before granting storage permission")
	}

	if foundObject == otherOptionBtn {
		// When the internet connection is unstable, it will shows the location selection prompt.
		// Dismiss it to do the following actions.
		confirmBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"privacy_continue"), androidui.ClassName(buttonClassName))
		if err := uiauto.NamedCombine("dismiss location prompt",
			cuj.FindAndClick(otherOptionBtn, defaultUITimeout),
			cuj.FindAndClick(confirmBtn, defaultUITimeout),
		)(ctx); err != nil {
			return err
		}
	}

	return m.grantStoragePermission(ctx)
}

// PlayVideoInFullScreen plays the video in full screen.
func (m *MxPlayerApp) PlayVideoInFullScreen(videoName string) uiauto.Action {
	return uiauto.Combine("play the video in full screen",
		m.DismissPrompts,
		m.EnterFullScreen,
		m.OpenAndPlayVideo(videoName),
		m.SetLoopOn,
	)
}

// OpenAndPlayVideo opens a video from Download folder on MX Player app and ensure the video is playing.
func (m *MxPlayerApp) OpenAndPlayVideo(videoName string) uiauto.Action {
	bufferingText := m.d.Object(androidui.ID(mxPlayerIDPrefix+"loading"), androidui.TextContains("Buffering"), androidui.ClassName(textClassName))
	return uiauto.NamedCombine("play video and ensure the video is playing",
		m.openVideoFromSearchResult(videoName),
		// Sometimes the notification prompt pops up after entering video page.
		// Dismiss it to do the following actions.
		m.dismissNotificationPrompt,
		// Videos with higher resolutions might need more time to buffer.
		uiauto.IfSuccessThen(cuj.WaitForExists(bufferingText, longUITimeout), cuj.WaitUntilGone(bufferingText, 30*time.Second)),
		m.skipHintButton,
		m.EnsurePlaying,
	)
}

func (m *MxPlayerApp) openVideoFromSearchResult(videoName string) uiauto.Action {
	videoTitle := m.d.Object(androidui.ID(mxPlayerIDPrefix+"title"), androidui.Text(videoName))
	searchText := m.d.Object(androidui.ID(mxPlayerIDPrefix+"search_src_text"), androidui.ClassName("android.widget.AutoCompleteTextView"))
	clickSearchButton := func(ctx context.Context) error {
		closeBtn := m.d.Object(androidui.PackageName(mxPlayerPackage), androidui.DescriptionContains("close"))
		androidCloseBtn := m.d.Object(androidui.PackageName("com.android.vending"), androidui.Description("Close"))
		searchBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"search"), androidui.ClassName(textClassName))
		foundObject, err := cuj.FindAnyExists(ctx, defaultUITimeout, closeBtn, androidCloseBtn, searchBtn)
		if err != nil {
			return errors.Wrap(err, "failed to find search button or close ads button")
		}
		if foundObject != searchBtn {
			if err := cuj.FindAndClick(foundObject, defaultUITimeout)(ctx); err != nil {
				return errors.Wrap(err, "failed to click close ads button")
			}
		}
		return uiauto.NamedCombine("click search button",
			cuj.FindAndClick(searchBtn, defaultUITimeout),
			cuj.FindAndClick(searchText, defaultUITimeout),
		)(ctx)
	}
	inputVideoName := uiauto.Combine("input video name",
		cuj.FindAndClick(searchText, defaultUITimeout),
		m.kb.AccelAction("Ctrl+A"),
		m.kb.TypeAction(videoName),
	)
	verifyVideoName := func(ctx context.Context) error {
		url, err := searchText.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get search text")
		}
		if url != videoName {
			testing.ContextLog(ctx, "Search text: ", videoName)
			return errors.Wrap(err, "failed to input correct video name")
		}
		return nil
	}
	ui := uiauto.New(m.tconn)
	return uiauto.NamedCombine("open video from search result",
		clickSearchButton,
		ui.RetryUntil(inputVideoName, verifyVideoName),
		m.kb.AccelAction("Enter"),
		ui.RetryUntil(cuj.FindAndClick(videoTitle, defaultUITimeout), cuj.WaitUntilGone(videoTitle, defaultUITimeout)),
	)
}

func (m *MxPlayerApp) skipHintButton(ctx context.Context) error {
	skipBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"tv_skip"), androidui.TextMatches("(Skip|Close)"), androidui.ClassName(textClassName))
	resumeBtn := m.d.Object(androidui.PackageName(mxPlayerPackage), androidui.Text("RESUME"), androidui.ClassName(buttonClassName))
	foundObject, err := cuj.FindAnyExists(ctx, defaultUITimeout, skipBtn, resumeBtn)
	if err != nil {
		// If there is no skip or resume button, we should just return.
		testing.ContextLog(ctx, "Skip or resume button does not appear: ", err)
		return nil
	}
	if err := cuj.FindAndClick(foundObject, defaultUITimeout)(ctx); err != nil {
		return errors.Wrap(err, "failed to click resume or skip button")
	}
	if foundObject == skipBtn {
		skipConfirmBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"dialog_conform"), androidui.Text("Skip"), androidui.ClassName(textClassName))
		if err := cuj.ClickIfExist(skipConfirmBtn, defaultUITimeout)(ctx); err != nil {
			return errors.Wrap(err, "failed to click skip confirm button")
		}
	}
	return nil
}

// CloseVideo closes the video and returns to home page.
func (m *MxPlayerApp) CloseVideo(videoName string) uiauto.Action {
	navigateUp := m.d.Object(androidui.PackageName(mxPlayerPackage), androidui.DescriptionMatches("(Navigate up|Back)"), androidui.ClassName(imageBtnClassName))
	videoTitle := m.d.Object(androidui.ID(mxPlayerIDPrefix+"title"), androidui.Text(videoName))
	return uiauto.NamedCombine("close the video and return to home page",
		// Pause the video to show ui layout.
		m.EnsurePaused,
		cuj.FindAndClick(navigateUp, defaultUITimeout),
		cuj.WaitForExists(videoTitle, defaultUITimeout),
		cuj.FindAndClick(navigateUp, defaultUITimeout),
		cuj.WaitUntilGone(videoTitle, defaultUITimeout),
	)
}

// EnterFullScreen switches Mx Player video to full screen.
func (m *MxPlayerApp) EnterFullScreen(ctx context.Context) error {
	if w, err := ash.GetARCAppWindowInfo(ctx, m.tconn, mxPlayerPackage); err != nil {
		return errors.Wrap(err, "failed to get ARC window info")
	} else if w.State == ash.WindowStateMaximized {
		return nil
	}

	if err := m.ChangeToResizable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizable")
	}

	if _, err := ash.SetARCAppWindowStateAndWait(ctx, m.tconn, mxPlayerPackage, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to set window state to full screen")
	}
	return nil
}

// ExitFullScreen exits Mx Player video from full screen.
func (m *MxPlayerApp) ExitFullScreen(ctx context.Context) error {
	if w, err := ash.GetARCAppWindowInfo(ctx, m.tconn, mxPlayerPackage); err != nil {
		return errors.Wrap(err, "failed to get ARC window info")
	} else if w.State == ash.WindowStateNormal {
		return nil
	}

	if err := m.ChangeToResizable(ctx); err != nil {
		return errors.Wrap(err, "failed to set window to resizable")
	}

	if _, err := ash.SetARCAppWindowStateAndWait(ctx, m.tconn, mxPlayerPackage, ash.WindowStateNormal); err != nil {
		return errors.Wrap(err, "failed to set window state to normal")
	}
	return nil
}

// EnsurePlaying ensures the video is playing now.
func (m *MxPlayerApp) EnsurePlaying(ctx context.Context) error {
	ui := uiauto.New(m.tconn)
	playBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"playpause"), androidui.ClassName(imageBtnClassName))
	return ui.Retry(retryTimes,
		uiauto.IfFailThen(cuj.WaitUntilGone(playBtn, defaultUITimeout),
			uiauto.NamedCombine("play video",
				cuj.FindAndClick(playBtn, defaultUITimeout),
				cuj.WaitUntilGone(playBtn, defaultUITimeout)),
		))(ctx)
}

// EnsurePaused ensures the video is paused now.
func (m *MxPlayerApp) EnsurePaused(ctx context.Context) error {
	ui := uiauto.New(m.tconn)
	playBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"playpause"), androidui.ClassName(imageBtnClassName))
	playerView := m.d.Object(androidui.ID(mxPlayerIDPrefix + "ui_layout"))
	return ui.Retry(retryTimes,
		uiauto.IfFailThen(cuj.WaitForExists(playBtn, defaultUITimeout),
			uiauto.NamedCombine("pause video",
				cuj.FindAndClick(playerView, defaultUITimeout),
				m.kb.AccelAction("Space"),
				cuj.WaitForExists(playBtn, defaultUITimeout)),
		))(ctx)
}

// SetLoopOn sets the video to loop.
func (m *MxPlayerApp) SetLoopOn(ctx context.Context) error {
	// Pause the video to show ui layout.
	if err := m.EnsurePaused(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure video is paused")
	}

	// There might be two different arc dump hierarchies that affect how nodes are captured.
	moreOptionsBtn := m.d.Object(androidui.PackageName(mxPlayerPackage), androidui.Description("More options"))
	moreBtn := m.d.Object(androidui.ClassName("android.widget.LinearLayout"), androidui.PackageName(mxPlayerPackage), androidui.Index(5), androidui.Clickable(true))
	foundObject, err := cuj.FindAnyExists(ctx, longUITimeout, moreOptionsBtn, moreBtn)
	if err != nil {
		return errors.Wrap(err, "failed to find 'more' or 'More options' button")
	}
	var setLoop uiauto.Action
	if foundObject == moreOptionsBtn {
		playOptionBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"title"), androidui.Text("Play"))
		loopOneBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"title"), androidui.Text("Loop one"))
		setLoop = uiauto.Combine("set loop on with 'More options' button",
			cuj.FindAndClick(playOptionBtn, defaultUITimeout),
			cuj.FindAndClick(loopOneBtn, defaultUITimeout),
		)
	} else {
		loopBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"description"), androidui.Text("Loop"))
		setLoop = uiauto.Combine("set loop on with 'More' button",
			cuj.FindAndClick(loopBtn, defaultUITimeout),
			cuj.FindAndClick(loopBtn, defaultUITimeout),
		)
	}

	return uiauto.NamedCombine("set loop on",
		cuj.FindAndClick(foundObject, defaultUITimeout),
		setLoop,
		m.EnsurePlaying,
	)(ctx)
}

// ChangeToResizable changes the MxPlayer window to resizable.
func (m *MxPlayerApp) ChangeToResizable(ctx context.Context) error {
	ui := uiauto.New(m.tconn)

	mxPlayerWindow := nodewith.Role(role.Window).Name(apps.MxPlayer.Name).First()
	centerBtn := nodewith.Role(role.Button).HasClass("FrameCenterButton").Ancestor(mxPlayerWindow)
	resizeBtn := nodewith.Role(role.Button).HasClass("FrameSizeButton").NameRegex(regexp.MustCompile("(Maximize|Restore)")).Ancestor(mxPlayerWindow)
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
func (m *MxPlayerApp) Close(ctx context.Context) error {
	return util.CloseApp(ctx, m.tconn, mxPlayerPackage)
}

func (m *MxPlayerApp) dismissNotificationPrompt(ctx context.Context) error {
	notificationMsg := m.d.Object(androidui.Text("Allow MX Player to send you notifications?"), androidui.ClassName(textClassName))
	if err := notificationMsg.WaitForExists(ctx, longUITimeout); err != nil {
		if androidui.IsTimeout(err) {
			return nil
		}
		return err
	}
	doNotAllowBtn := m.d.Object(androidui.ID(permissionIDPrefix+"permission_deny_button"), androidui.Text("Don’t allow"))
	return uiauto.NamedCombine("dismiss notification prompt",
		cuj.FindAndClick(doNotAllowBtn, defaultUITimeout),
		cuj.WaitUntilGone(notificationMsg, defaultUITimeout),
	)(ctx)
}

func (m *MxPlayerApp) grantStoragePermission(ctx context.Context) error {
	openSettingsBtn := m.d.Object(androidui.ID(mxPlayerIDPrefix+"storage_permission_accept"), androidui.Text("OPEN SETTINGS"))
	allowToggle := m.d.Object(androidui.ResourceID("android:id/switch_widget"))
	navigateUp := m.d.Object(androidui.PackageName("com.android.settings"), androidui.DescriptionMatches("(Navigate up|Back)"), androidui.ClassName(imageBtnClassName))
	allowAccessToFiles := uiauto.Combine("allow access to files",
		cuj.FindAndClick(openSettingsBtn, defaultUITimeout),
		cuj.WaitUntilGone(openSettingsBtn, longUITimeout),
		cuj.FindAndClick(allowToggle, defaultUITimeout),
		cuj.FindAndClick(navigateUp, defaultUITimeout),
		cuj.WaitUntilGone(allowToggle, defaultUITimeout),
	)

	allowBtn := m.d.Object(androidui.ResourceIDMatches(".*permission_allow_button$"), androidui.TextMatches("(?i)Allow"))
	return uiauto.NamedCombine("grant storage permission",
		uiauto.IfSucceedThenElse(cuj.WaitForExists(openSettingsBtn, defaultUITimeout),
			allowAccessToFiles,
			cuj.FindAndClick(allowBtn, defaultUITimeout)),
	)(ctx)
}
