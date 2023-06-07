// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	youtubePkg       = "com.google.android.youtube"
	noThanksText     = "NO THANKS"
	playBtnDesc      = "Play video"
	pauseBtnDesc     = "Pause video"
	playPauseBtnID   = youtubePkg + ":id/player_control_play_pause_replay_button"
	playerViewID     = youtubePkg + ":id/player_view"
	moreOptionsID    = youtubePkg + ":id/player_overflow_button"
	optionsID        = youtubePkg + ":id/bottom_sheet_list"
	dismissID        = youtubePkg + ":id/dismiss"
	qualityMessageID = youtubePkg + ":id/message"
	optionsClassName = "android.view.ViewGroup"
	uiWaitTime       = 5 * time.Second // this is for arc-obj, not for uiauto.Context
	retryTimes       = 3
)

var appStartTime time.Duration

// YtApp defines the members related to youtube app.
type YtApp struct {
	cr            *chrome.Chrome
	tconn         *chrome.TestConn
	kb            *input.KeyboardEventWriter
	a             *arc.ARC
	d             *androidui.Device
	act           *arc.Activity
	outDir        string
	youtubeApkURL string
	premium       bool // Indicate if the account is premium.
}

// NewYtApp creates an instance of YtApp.
func NewYtApp(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *androidui.Device, outDir, youtubeApkURL string) *YtApp {
	return &YtApp{
		cr:            cr,
		tconn:         tconn,
		kb:            kb,
		a:             a,
		d:             d,
		outDir:        outDir,
		youtubeApkURL: youtubeApkURL,
		premium:       true,
	}
}

// Install installs the Youtube app using the apk.
func (y *YtApp) Install(ctx context.Context) error {
	// If youtubeApkURL is an empty string, it means install the latest youtube app from the play store.
	if y.youtubeApkURL == "" {
		if err := playstore.InstallOrUpdateAppAndClose(ctx, y.tconn, y.a, y.d, youtubePkg, &playstore.Options{TryLimit: -1}); err != nil {
			return errors.Wrap(err, "failed to install Youtube app from playstore")
		}
		appVersion, _, err := dumpAppInfo(ctx, y.a, y.d, youtubePkg)
		if err != nil {
			return errors.Wrapf(err, "failed to get %s version: %v", youtubePkg, appVersion)
		}
		return nil
	}
	installed, err := y.a.PackageInstalled(ctx, youtubePkg)
	if err != nil {
		return err
	}
	apkFileName := path.Base(y.youtubeApkURL)
	knownGoodVersionCode, err := parseKnownGoodVersionCode(apkFileName)
	if err != nil {
		return err
	}
	testing.ContextLog(ctx, "Known good version code: ", knownGoodVersionCode)
	if installed {
		appVersion, appVersionCode, err := dumpAppInfo(ctx, y.a, y.d, youtubePkg)
		if err != nil {
			return errors.Wrapf(err, "failed to get %s version: %v", youtubePkg, appVersion)
		}
		if appVersionCode == knownGoodVersionCode {
			return nil
		}
		testing.ContextLog(ctx, "Uninstall the Youtube app")
		if err := y.a.Uninstall(ctx, youtubePkg); err != nil {
			return errors.Wrapf(err, "failed to uninstall %s version", youtubePkg)
		}
	}
	downloadPath := filepath.Join(os.TempDir(), apkFileName)
	if err := downloadToLocalFile(y.youtubeApkURL, downloadPath); err != nil {
		return err
	}
	defer os.Remove(downloadPath)
	return y.a.Install(ctx, downloadPath)
}

// Uninstall uninstalls the Youtube app if it is installed.
// This function does nothing if the app is initially uninstalled.
func (y *YtApp) Uninstall(ctx context.Context) error {
	installed, err := y.a.PackageInstalled(ctx, youtubePkg)
	if err != nil {
		return errors.Wrap(err, "failed to get package install status")
	}
	if !installed {
		testing.ContextLog(ctx, "The app is already uninstalled")
		return nil
	}
	return y.a.Uninstall(ctx, youtubePkg)
}

// parseKnownGoodVersionCode parses known good version code by apk filename.
// The correct format for the apk filename is "youtube_<versioncode>_<date>".
func parseKnownGoodVersionCode(apkFileName string) (string, error) {
	filenameSplit := strings.Split(apkFileName, "_")
	if len(filenameSplit) == 3 {
		knownGoodVersionCode := filenameSplit[1]
		return knownGoodVersionCode, nil
	}
	return "", errors.Errorf("APK file name is incorrect got %s; want youtube_<versioncode>_<date>.apk", apkFileName)
}

func downloadToLocalFile(url, downloadPath string) error {
	dest, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download failed: %s", resp.Status)
	}

	if _, err := io.Copy(dest, resp.Body); err != nil {
		return err
	}
	return nil
}

// Launch opens Youtube app and return error if it failed.
func (y *YtApp) Launch(ctx context.Context) (err error) {
	const (
		youtubeApp              = "Youtube App"
		youtubeAct              = "com.google.android.apps.youtube.app.WatchWhileActivity"
		youtubeLogoDescription  = "YouTube Premium"
		accountImageDescription = "Account"
		accountImageID          = youtubePkg + ":id/image"
	)

	if appStartTime, y.act, err = cuj.OpenAppAndGetStartTime(ctx, y.tconn, y.a, youtubePkg, youtubeApp, youtubeAct); err != nil {
		return errors.Wrap(err, "failed to get app start time")
	}

	dismissMobilePrompt := func(ctx context.Context) error {
		// The "This app is designed for mobile" prompt needs to be dismissed to get to the log in page.
		if err := apputil.DismissMobilePrompt(ctx, y.tconn); err != nil {
			return errors.Wrap(err, `failed to dismiss "This app is designed for mobile" prompt`)
		}
		return nil
	}

	checkAccountState := func(ctx context.Context) error {
		accountImage := y.d.Object(androidui.ID(accountImageID), androidui.DescriptionContains(accountImageDescription))
		if err := accountImage.WaitForExists(ctx, uiWaitTime); err != nil {
			return errors.Wrap(err, "failed to check for Youtube app launched")
		}

		premiumLogo := y.d.Object(androidui.Description(youtubeLogoDescription))
		if err := premiumLogo.WaitForExists(ctx, uiWaitTime); err != nil {
			y.premium = false
			testing.ContextLog(ctx, "Current account is free account")
		}
		return nil
	}

	return uiauto.Combine("dismiss prompt and check if it is free account",
		dismissMobilePrompt,
		DismissPrompts(y.d),
		checkAccountState,
	)(ctx)
}

// SearchAndPlayVideo search for a given video and play it.
func (y *YtApp) SearchAndPlayVideo(ctx context.Context, video VideoSrc) error {
	testing.ContextLog(ctx, "Search and play video")
	const (
		searchButtonID   = youtubePkg + ":id/menu_item_1"
		searchEditTextID = youtubePkg + ":id/search_edit_text"
		resultsViewID    = youtubePkg + ":id/results"
	)

	searchButton := y.d.Object(androidui.ID(searchButtonID))
	if err := cuj.FindAndClick(searchButton, 2*uiWaitTime)(ctx); err != nil {
		return errors.Wrap(err, "failed to click search button")
	}

	searchEditText := y.d.Object(androidui.ID(searchEditTextID))
	if err := cuj.FindAndClick(searchEditText, uiWaitTime)(ctx); err != nil {
		return errors.Wrap(err, "failed to find 'searchTextfield'")
	}

	inputURL := uiauto.Combine("input video url",
		y.kb.AccelAction("Ctrl+A"),
		y.kb.TypeAction(video.URL),
	)

	verifyURL := func(ctx context.Context) error {
		url, err := searchEditText.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get search text")
		}
		if url != video.URL {
			testing.ContextLog(ctx, "Search text: ", url)
			return errors.Wrap(err, "failed to input correct video url")
		}
		return nil
	}

	ui := uiauto.New(y.tconn)
	if err := uiauto.NamedCombine("search video",
		ui.RetryUntil(inputURL, verifyURL),
		y.kb.AccelAction("Enter"),
	)(ctx); err != nil {
		return err
	}

	resultsView := y.d.Object(androidui.ID(resultsViewID))
	if err := resultsView.WaitForExists(ctx, uiWaitTime); err != nil {
		return errors.Wrap(err, "failed to find the results from video URL")
	}

	firstVideo := y.d.Object(androidui.DescriptionContains(video.Title))
	startTime := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {

		if err := cuj.FindAndClick(firstVideo, uiWaitTime)(ctx); err != nil {
			if strings.Contains(err.Error(), "click") {
				return testing.PollBreak(err)
			}
			return errors.Wrap(err, "failed to find 'First Video'")
		}

		testing.ContextLogf(ctx, "Elapsed time when waiting the video list: %.3f s", time.Since(startTime).Seconds())
		return nil
	}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to click first video")
	}

	// It has been seen that low-end DUTs sometimes can take as much as 10-20 seconds to finish loading after clicking
	// on a video from the search results. Logic is added here to wait for the loading to complete before proceeding to
	// prevent unexpected errors.
	if err := y.waitForLoadingComplete(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for loading to complete")
	}

	return nil
}

// StartFromBeginning starts the video to the beginning by dragging
// the current time node to the most left of the bar.
func (y *YtApp) StartFromBeginning(ctx context.Context) error {
	timeBar := y.d.Object(androidui.ClassName("android.widget.SeekBar"), androidui.DescriptionContains("minutes"))
	timeBarBounds, err := timeBar.GetBounds(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get node bounds")
	}

	return apputil.DragAndDrop(y.a, timeBarBounds.CenterPoint(), timeBarBounds.LeftCenter(), time.Second)(ctx)
}

// OpenAndPlayVideo opens a video on youtube app.
func (y *YtApp) OpenAndPlayVideo(video VideoSrc) uiauto.Action {
	return func(ctx context.Context) (err error) {
		testing.ContextLog(ctx, "Open Youtube app")

		if err = y.Launch(ctx); err != nil {
			return errors.Wrap(err, "failed to launch the app")
		}

		if err := y.SearchAndPlayVideo(ctx, video); err != nil {
			return errors.Wrap(err, "failed to play video")
		}

		if err := y.SwitchQuality(ctx, video.Quality); err != nil {
			return errors.Wrap(err, "failed to switch Quality")
		}

		return nil
	}
}

// SwitchQuality switches the video quality by continuous action.
// Due to the different response time of DUTs.
// We need to combine these actions in Poll to make switch quality works smoothly.
func (y *YtApp) SwitchQuality(ctx context.Context, quality Quality) error {
	testing.ContextLogf(ctx, "Switch Quality to %q", quality)

	const (
		// advancedButtonIndex is the index of "Advanced" button shown in "Quality" list.
		advancedButtonIndex = 3
		// qualityButtonIndex is the index of "Quality" button shown in "Settings" list.
		qualityButtonIndex = 0

		qualityText                   = "Quality"
		advancedText                  = "Advanced"
		moreOptionsText               = "More options"
		qualityUnavailableMessageText = "Quality unavailable"
	)

	if err := y.skipAds(ctx); err != nil {
		return errors.Wrap(err, "failed to skip YouTube ads")
	}
	if err := y.skipSurvey(ctx); err != nil {
		return errors.Wrap(err, "failed to skip YouTube survey")
	}

	clickMoreOptions := func(ctx context.Context) error {
		moreBtn := y.d.Object(androidui.ID(moreOptionsID))
		playerView := y.d.Object(androidui.ID(playerViewID))
		// 1. The playerView cannot be found/clicked when the options dialog (used for selecting quality) is present.
		// Therefore, press "Esc" to close the options dialog, then click on the video to reveal "More Options".
		// 2. If the "More Options" button is not on the display, pressing "Esc" will bring it up directly.
		return uiauto.NamedCombine(fmt.Sprintf("click %q button", moreOptionsText),
			y.kb.AccelAction("Esc"),
			uiauto.IfFailThen(moreBtn.Exists, cuj.FindAndClick(playerView, uiWaitTime)),
			cuj.FindAndClick(moreBtn, uiWaitTime),
		)(ctx)
	}

	captureScreenshot := func(fileName string) action.Action {
		return func(context.Context) error {
			if err := screenshot.CaptureChrome(ctx, y.cr, filepath.Join(y.outDir, fileName)); err != nil {
				testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
			}
			return nil
		}
	}

	clickAdvancedButton := func(context.Context) error {
		// Sometimes clicking `Quality` button will show "Quality unavailable"
		// message and changing quality will not be allowed.
		qualityUnavailableText := y.d.Object(androidui.ID(qualityMessageID), ui.Text(qualityUnavailableMessageText))
		if err := qualityUnavailableText.WaitForExists(ctx, 3*time.Second); err == nil {
			return errors.Errorf("%q message is shown, cannot change quality", qualityUnavailableMessageText)
		}
		// There might be two different arc dump hierarchies that affect how nodes are captured.
		var advancedButton *ui.Object
		options := y.d.Object(androidui.ID(optionsID))
		if err := options.Exists(ctx); err == nil {
			testing.ContextLog(ctx, "Select advanced button with class name: ", optionsClassName)
			advancedButton = y.d.Object(androidui.ClassName(optionsClassName), androidui.Index(advancedButtonIndex), androidui.Clickable(true))
		} else {
			testing.ContextLog(ctx, "Select advanced button with text: ", advancedText)
			advancedButton = y.d.Object(androidui.Text(advancedText))
		}
		if err := cuj.FindAndClick(advancedButton, uiWaitTime)(ctx); err != nil {
			return errors.Wrap(err, "failed to find/click advanced button")
		}
		return nil
	}

	qualityButton := y.d.Object(androidui.ClassName(optionsClassName), androidui.Index(qualityButtonIndex), androidui.Clickable(true))
	return uiauto.Retry(retryTimes, uiauto.NamedCombine("switch video quality to "+string(quality),
		clickMoreOptions,
		captureScreenshot("before-click-quality.png"),
		cuj.FindAndClick(qualityButton, uiWaitTime),
		captureScreenshot("after-click-quality.png"),
		clickAdvancedButton,
		ClickQualityOption(y.d, y.cr, y.outDir, quality),
	))(ctx)
}

// ClickQualityOption clicks the desired video quality option.
func ClickQualityOption(d *ui.Device, cr *chrome.Chrome, outDir string, quality Quality) action.Action {
	return func(ctx context.Context) error {
		var qualityOption *ui.Object
		options := d.Object(androidui.ID(optionsID))
		if err := options.Exists(ctx); err == nil {
			qualityList := d.Object(androidui.ID(optionsID))
			qualityView := d.Object(androidui.ClassName(optionsClassName))
			if err := qualityList.GetChild(ctx, qualityView); err != nil {
				return errors.Wrapf(err, "failed to get %+v child", qualityList)
			}
			qualityOptionsCount, err := qualityView.GetChildCount(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get the count of quality options")
			}
			qualityPosition, ok := qualityOptionMap[quality]
			if !ok {
				return errors.Wrapf(err, "failed to parse quality at %q from map: %v", quality, qualityOptionMap)
			}
			if qualityOptionsCount < qualityPosition {
				return errors.Errorf("quality(%s) is unavailable", quality)
			}
			targetQualityIndex := qualityOptionsCount - qualityPosition
			testing.ContextLog(ctx, "Select target quality button with class name at index: ", targetQualityIndex)
			qualityOption = d.Object(androidui.ClassName(optionsClassName), androidui.Index(targetQualityIndex), androidui.Clickable(true))
		} else {
			testing.ContextLog(ctx, "Select target quality button with text: ", quality)
			qualityListItemID := youtubePkg + ":id/list_item_text"
			qualityOption = d.Object(androidui.ID(qualityListItemID), androidui.Text(string(quality)))
		}
		// There might be 60 and 30 fps options shown in the same time.
		// Capture screenshot of the quality options list for further debugging.
		if err := screenshot.CaptureChrome(ctx, cr, filepath.Join(outDir, "quality_list.png")); err != nil {
			testing.ContextLog(ctx, "Failed to capture screenshot for the quality options list")
		}
		if err := cuj.FindAndClick(qualityOption, uiWaitTime)(ctx); err != nil {
			return errors.Wrap(err, "failed to find/click the quality option")
		}

		qualityMessageText := d.Object(androidui.ID(qualityMessageID), ui.TextContains(string(quality)))
		if err := qualityMessageText.WaitUntilGone(ctx, uiWaitTime); err != nil {
			return errors.Wrap(err, "failed to wait the quality message gone")
		}
		return nil
	}
}

// SetLoopVideo sets loop the video by continuous action.
// Due to the different response time of DUTs.
// We need to combine these actions in Poll to make switch quality works smoothly.
func (y *YtApp) SetLoopVideo(ctx context.Context) error {
	testing.ContextLog(ctx, "Set loop the video")

	const (
		// loopButtonIndex is the index of "Loop" button shown in "Settings" list.
		loopButtonIndex = 2

		loopText         = "Loop video"
		moreOptionsText  = "More options"
		optionsClassName = "android.support.v7.widget.RecyclerView"
		loopClassName    = "android.view.ViewGroup"
		moreOptions      = youtubePkg + ":id/player_overflow_button"
		loopListItemID   = youtubePkg + ":id/list_item_text"
	)

	moreBtn := y.d.Object(androidui.ID(moreOptionsID))
	playerView := y.d.Object(androidui.ID(playerViewID))
	loopButton := y.d.Object(androidui.ClassName(loopClassName), androidui.Index(loopButtonIndex), androidui.Clickable(true))
	setLoopVideoOn := uiauto.NamedCombine("set loop video on",
		y.kb.AccelAction("Esc"),
		uiauto.IfFailThen(moreBtn.Exists, cuj.FindAndClick(playerView, uiWaitTime)),
		cuj.FindAndClick(moreBtn, uiWaitTime),
		cuj.FindAndClick(loopButton, uiWaitTime),
	)
	return uiauto.Retry(retryTimes, setLoopVideoOn)(ctx)
}

func (y *YtApp) waitForLoadingComplete(ctx context.Context) error {
	const (
		titleID               = youtubePkg + ":id/title"
		shareBtnText          = "Share"
		shareBtnTextID        = youtubePkg + ":id/button_text"
		watchListID           = youtubePkg + ":id/watch_list"
		alternateElementClass = "android.view.ViewGroup"
		alternateTitleDesc    = "Expand description"
		alternateExpandDesc   = "Expand Mini Player"
	)
	videoTitle := y.d.Object(androidui.ID(titleID))
	shareBtn := y.d.Object(androidui.Text(shareBtnText), androidui.ID(shareBtnTextID))
	watchList := y.d.Object(androidui.ID(watchListID))
	// An alternate video title and share button are added here to support the two versions of UI trees observed across DUTs.
	// For details, please refer to b/206011393.
	alternateVideoTitle := y.d.Object(androidui.ClassName(alternateElementClass), androidui.Description(alternateTitleDesc))
	alternateExpandMenu := y.d.Object(androidui.ClassName(alternateElementClass), androidui.Description(alternateExpandDesc))
	alternateShareBtn := y.d.Object(androidui.ClassName(alternateElementClass), androidui.Description(shareBtnText))

	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := videoTitle.Exists(ctx); err != nil {
			if alternateVideoTitle.Exists(ctx) != nil && alternateExpandMenu.Exists(ctx) != nil {
				return errors.New("failed to wait for video title to render")
			}
		}
		if err := shareBtn.Exists(ctx); err != nil {
			if err2 := alternateShareBtn.Exists(ctx); err2 != nil {
				return errors.New("failed to wait for share button to render")
			}
		}
		if err := watchList.Exists(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for watch list to render")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute})
}

func (y *YtApp) isPremiumAccount() bool {
	return y.premium
}

func (y *YtApp) checkYoutubeAppPIP(ctx context.Context) error {
	testing.ContextLog(ctx, "Check window state should be PIP")
	startTime := time.Now()

	ws, err := ash.GetARCAppWindowState(ctx, y.tconn, youtubePkg)
	if err != nil {
		return errors.Wrap(err, "can not get ARC App Window State")
	}
	if ws == ash.WindowStatePIP {
		testing.ContextLogf(ctx, "Elapsed time when checking PIP mode: %.3f s", time.Since(startTime).Seconds())
		return nil
	}

	waitForPipMode := func(ctx context.Context) error {
		return ash.WaitForARCAppWindowState(ctx, y.tconn, youtubePkg, ash.WindowStatePIP)
	}

	// Checking PIP mode sometimes doesn't work (e.g. if chrome window is not in fullscreen),
	// retry a few times to enable PIP mode.
	return uiauto.Retry(3,
		uiauto.Combine("change to pip mode",
			y.kb.AccelAction("Alt+="),
			waitForPipMode,
		),
	)(ctx)
}

type fullscreenState string

const (
	enterFullscreen fullscreenState = "Enter fullscreen"
	exitFullscreen  fullscreenState = "Exit fullscreen"
)

// EnterFullScreen switches youtube video to full screen.
func (y *YtApp) EnterFullScreen(ctx context.Context) error {
	// If the youtube app is already in full screen, skip the process to go fullscreen.
	if isFullScreen, err := y.isFullscreen(ctx); err != nil {
		return err
	} else if isFullScreen {
		return nil
	}

	playerView := y.d.Object(androidui.ID(playerViewID))
	enterFsBtn := y.d.Object(androidui.Description(string(enterFullscreen)))

	return uiauto.Retry(retryTimes, uiauto.NamedCombine("enter Youtube to fullscreen",
		uiauto.IfFailThen(enterFsBtn.Exists, cuj.FindAndClick(playerView, uiWaitTime)),
		cuj.FindAndClick(enterFsBtn, uiWaitTime),
		waitWindowStateFullscreen(y.tconn, YoutubeWindowTitle),
	))(ctx)
}

// ExitFullScreen exits Youtube video from fullscreen.
func (y *YtApp) ExitFullScreen(ctx context.Context) error {
	// If the youtube app is already not in full screen, skip the process to exit fullscreen.
	if isFullScreen, err := y.isFullscreen(ctx); err != nil {
		return err
	} else if !isFullScreen {
		return nil
	}

	playerView := y.d.Object(androidui.ID(playerViewID))
	exitFsBtn := y.d.Object(androidui.Description(string(exitFullscreen)))

	return uiauto.Retry(retryTimes, uiauto.Combine("exit Youtube from fullscreen",
		uiauto.IfFailThen(exitFsBtn.Exists, cuj.FindAndClick(playerView, uiWaitTime)),
		cuj.FindAndClick(exitFsBtn, uiWaitTime),
		waitWindowStateExitFullscreen(y.tconn, YoutubeWindowTitle),
	))(ctx)
}

func (y *YtApp) isFullscreen(ctx context.Context) (bool, error) {
	enterFsBtn := y.d.Object(androidui.Description(string(enterFullscreen)))
	exitFsBtn := y.d.Object(androidui.Description(string(exitFullscreen)))
	var controlBtn *androidui.Object
	if err := uiauto.Retry(retryTimes, func(context.Context) (err error) {
		if err := y.showMoreOptionsButton(ctx); err != nil {
			return err
		}
		// The settings buttons might have happened to disappear, so none of them could be found.
		controlBtn, err = cuj.FindAnyExists(ctx, uiWaitTime, enterFsBtn, exitFsBtn)
		if err != nil {
			return err
		}
		return nil
	})(ctx); err != nil {
		return false, err
	}
	if controlBtn == enterFsBtn {
		return false, nil
	}
	return true, nil
}

// showMoreOptionsButton checks if the "More Options" button is not on the display, click on the video to bring it up.
func (y *YtApp) showMoreOptionsButton(ctx context.Context) (err error) {
	if err := y.skipAds(ctx); err != nil {
		return errors.Wrap(err, "failed to skip YouTube ads")
	}

	moreBtn := y.d.Object(androidui.ID(moreOptionsID))
	playerView := y.d.Object(androidui.ID(playerViewID))
	return uiauto.IfFailThen(moreBtn.Exists,
		// It might not successfully call out the buttons on the low-end DUTs.
		uiauto.Retry(retryTimes, uiauto.NamedCombine("click video to bring up setting buttons",
			cuj.FindAndClick(playerView, uiWaitTime),
			cuj.WaitForExists(moreBtn, uiWaitTime),
		)),
	)(ctx)
}

// PlayVideo plays video on youtube app.
func (y *YtApp) PlayVideo(ctx context.Context) error {
	testing.ContextLog(ctx, "Play video")

	pauseBtn := y.d.Object(androidui.ID(playPauseBtnID), androidui.Description(pauseBtnDesc))
	playBtn := y.d.Object(androidui.ID(playPauseBtnID), androidui.Description(playBtnDesc))
	return uiauto.Retry(retryTimes, func(context.Context) (err error) {
		playPauseBtn, err := cuj.FindAnyExists(ctx, uiWaitTime, playBtn, pauseBtn)
		if err != nil {
			return err
		}
		if playPauseBtn == pauseBtn {
			return nil
		}
		if err := playBtn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click the play button")
		}
		if err := pauseBtn.WaitForExists(ctx, uiWaitTime); err != nil {
			return errors.Wrapf(err, "failed to find the pause button in %s", uiWaitTime)
		}
		return nil
	})(ctx)
}

// PauseAndPlayVideo verifies video playback on youtube app.
func (y *YtApp) PauseAndPlayVideo(ctx context.Context) error {
	testing.ContextLog(ctx, "Pause and play video")
	const sleepTime = 3 * time.Second

	playerView := y.d.Object(androidui.ID(playerViewID))
	pauseBtn := y.d.Object(androidui.ID(playPauseBtnID), androidui.Description(pauseBtnDesc))
	playBtn := y.d.Object(androidui.ID(playPauseBtnID), androidui.Description(playBtnDesc))

	// The video should be playing at this point. However, we'll double check to make sure
	// as we have seen a few cases where the video became paused automatically.
	if err := y.ensureVideoPlaying(ctx, playerView, playBtn); err != nil {
		return errors.Wrap(err, "failed to ensure video is playing before pausing")
	}

	startTime := time.Now()
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := cuj.FindAndClick(playerView, uiWaitTime)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find/click the player view in %s", uiWaitTime)
		}

		if err := cuj.FindAndClick(pauseBtn, uiWaitTime)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find/click the pause button in %s", uiWaitTime)
		}

		if err := playBtn.WaitForExists(ctx, 2*time.Second); err != nil {
			return errors.Wrap(err, "failed to find the play button in 2s")
		}

		if err := y.PlayVideo(ctx); err != nil {
			return errors.Wrap(err, "failed to play video")
		}

		// GoBigSleepLint: Keep the video playing for a short time.
		if err := testing.Sleep(ctx, sleepTime); err != nil {
			return errors.Wrap(err, "failed to sleep while video is playing")
		}

		testing.ContextLogf(ctx, "Elapsed time when checking the playback status of youtube app: %.3f s", time.Since(startTime).Seconds())
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 2 * time.Minute})
}

func (y *YtApp) ensureVideoPlaying(ctx context.Context, playerView, playBtn *androidui.Object) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		const (
			loadingViewID        = youtubePkg + ":id/player_loading_view_thin"
			loadingViewClassName = "android.widget.ProgressBar"
		)
		loadingView := y.d.Object(androidui.ClassName(loadingViewClassName), androidui.ID(loadingViewID))
		if err := loadingView.Exists(ctx); err == nil {
			return errors.New("Video is loading")
		}

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

// IsPlaying checks if the video is playing now.
func (y *YtApp) IsPlaying() uiauto.Action {
	return func(ctx context.Context) error {
		playerView := y.d.Object(androidui.ID(playerViewID))
		playBtn := y.d.Object(androidui.ID(playPauseBtnID), androidui.Description(playBtnDesc))
		testing.ContextLog(ctx, "Verify the video is playing")
		return y.ensureVideoPlaying(ctx, playerView, playBtn)
	}
}

// DismissPrompts dismisses prompts if exist.
func DismissPrompts(d *ui.Device) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Dismiss trial prompt")
		const (
			androidUpdateID       = "com.android.vending:id/0_resource_name_obfuscated"
			androidNotificationID = "com.android.permissioncontroller:id/permission_deny_button"
			doNotAllowText        = "Don’t allow"
			skipTrialTextReg      = "(?i)Skip trial"
			closeDescription      = "Close"
		)

		// Skip the "Update Youtube?" popup since the last known good version
		// might not be the latest version.
		noThanksButton := d.Object(androidui.ID(androidUpdateID), androidui.Text(noThanksText))
		// Dismiss the "Allow YouTube to send you notifications?" popup.
		doNotAllowButton := d.Object(androidui.ID(androidNotificationID), androidui.Text(doNotAllowText))
		skipTrial := d.Object(androidui.ID(dismissID), androidui.TextMatches(skipTrialTextReg))
		closeButton := d.Object(androidui.Description(closeDescription))
		noThanksEle := d.Object(androidui.ID(dismissID), androidui.Text(noThanksText))
		promptList := []*androidui.Object{noThanksButton, doNotAllowButton, skipTrial, closeButton, noThanksEle}
		timeout := time.Duration(len(promptList)) * uiWaitTime
		return testing.Poll(ctx, func(ctx context.Context) error {
			closeBtn, err := cuj.FindAnyExists(ctx, uiWaitTime, promptList...)
			if err != nil {
				return nil
			}
			if err := closeBtn.Click(ctx); err != nil {
				return errors.Wrap(err, "failed to click close button")
			}
			return errors.New("dismiss prompts")
		}, &testing.PollOptions{Timeout: timeout})
	}
}

// dumpAppInfo dumps Youtube app version name and code.
func dumpAppInfo(ctx context.Context, a *arc.ARC, d *ui.Device, appPkgName string) (string, string, error) {
	var versionName, versionCode string
	out, err := a.Command(ctx, "dumpsys", "package", appPkgName).Output(testexec.DumpLogOnError)
	if err == nil {
		versionNamePrefix, versionCodePrefix := "versionName=", "versionCode="
		output := string(out)
		splitOutput := strings.Split(output, "\n")
		for splitLine := range splitOutput {
			if strings.Contains(splitOutput[splitLine], versionNamePrefix) {
				versionName = strings.Split(splitOutput[splitLine], "=")[1]
			}
			if strings.Contains(splitOutput[splitLine], versionCodePrefix) {
				versionCodeLine := strings.Fields(splitOutput[splitLine])[0]
				versionCode = strings.Split(versionCodeLine, "=")[1]
			}
			if versionCode != "" && versionName != "" {
				break
			}
		}
		if versionName == "" {
			err = errors.Errorf("%s is not found in the output", versionNamePrefix)
		}
		if versionCode == "" {
			err = errors.Errorf("%s is not found in the output", versionCodePrefix)
		}
	}
	testing.ContextLogf(ctx, "Youtube app version: %s; Version code: %s", versionName, versionCode)
	return versionName, versionCode, err
}

func (y *YtApp) skipAds(ctx context.Context) error {
	if y.premium {
		testing.ContextLog(ctx, "Currently using Premium account; no need to check for ads")
		return nil
	}

	const (
		visitAdvertiserText = "Visit advertiser"
		skipAdsID           = youtubePkg + ":id/skip_ad_button"
	)

	visitAdvertiserBtn := y.d.Object(androidui.Text(visitAdvertiserText))
	skipAdsBtn := y.d.Object(androidui.ID(skipAdsID))

	testing.ContextLog(ctx, "Checking for YouTube ads")
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := visitAdvertiserBtn.WaitForExists(ctx, uiWaitTime); err != nil && androidui.IsTimeout(err) {
			return nil
		}
		if err := skipAdsBtn.Exists(ctx); err != nil {
			return errors.Wrap(err, "'Skip ads' button not available yet")
		}
		if err := skipAdsBtn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Skip ads'")
		}
		return errors.New("have not determined whether the ad has been skipped successfully")
	}, &testing.PollOptions{Timeout: time.Minute})
}

func (y *YtApp) skipSurvey(ctx context.Context) error {
	testing.ContextLog(ctx, "Checking for YouTube survey")

	const skipSurveyID = youtubePkg + ":id/skip_button"
	skipSurveyBtn := y.d.Object(androidui.ID(skipSurveyID))
	return cuj.ClickIfExist(skipSurveyBtn, uiWaitTime)(ctx)
}

// Close closes the resources related to video.
func (y *YtApp) Close(ctx context.Context) {
	if y.act != nil {
		y.act.Stop(ctx, y.tconn)
		y.act.Close(ctx)
		y.act = nil
	}
}
