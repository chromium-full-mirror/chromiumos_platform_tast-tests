// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package multitaskingapp contains the test code for power.MultiTaskingApp test.
package multitaskingapp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/arcvideoplayback"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/socialapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParams holds the parameters to run the test main logic.
type TestParams struct {
	BrowserType   browser.Type
	WebSource     cuj.WebSourceType
	TestName      string
	OutDir        string
	ElementAPKURL string
	DataPath      func(string) string
	TabletMode    bool
	BrowserTime   time.Duration
	SocialAppTime time.Duration
	VideoAppTime  time.Duration
}

// TestResources holds the resources that would be needed in the test.
type TestResources struct {
	Cr        *chrome.Chrome
	Kb        *input.KeyboardEventWriter
	UIHandler cuj.UIActionHandler
	Tconn     *chrome.TestConn
	Btconn    *chrome.TestConn
	A         *arc.ARC
	Br        *browser.Browser
}

// urlData web page for browsing test
type urlData struct {
	// Number of page to test
	NumPage int `json:"num_page"`
	// Version of page caching, live indicates live page.
	Version string `json:"version"`
	// Pages to browse
	Pages []string `json:"pages"`
}

// browsingConfig describes configuration for this test.
type browsingConfig struct {
	Version string  `json:"config_version"`
	URLData urlData `json:"url_data"`
}

const (
	// VideoSrc is the video used for this test.
	VideoSrc  = "multitaskingapp/vp9_720_60fps.webm"
	videoName = "vp9_720_60fps"
)

// Run runs the MultitaskingApp test.
func Run(ctx context.Context, resources *TestResources, params *TestParams) (retErr error) {

	var (
		bt            = params.BrowserType
		outDir        = params.OutDir
		testName      = params.TestName
		elementAPKURL = params.ElementAPKURL
		dataPath      = params.DataPath
		browserTime   = params.BrowserTime
		socialAppTime = params.SocialAppTime
		videoAppTime  = params.VideoAppTime
		cr            = resources.Cr
		tconn         = resources.Tconn
		a             = resources.A
		kb            = resources.Kb
		br            = resources.Br
		bTconn        = resources.Btconn
		uiHandler     = resources.UIHandler
	)

	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find browser app info")
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create new ARC device")
	}

	defer func(ctx context.Context) {
		if d.Alive(ctx) {
			testing.ContextLog(ctx, "UI device is still alive")
			d.Close(ctx)
		}
	}(closeCtx)

	splitAccount := strings.Split(cr.Creds().User, "@")
	if len(splitAccount) < 1 {
		return errors.Wrapf(err, "failed to get username from the account: %s", cr.Creds().User)
	}
	username := splitAccount[0]
	socialApp := socialapp.NewElement(tconn, kb, a, d, username, elementAPKURL)
	if err := socialApp.Install(ctx); err != nil {
		return errors.Wrap(err, "failed to install social app")
	}
	defer socialApp.Uninstall(closeCtx)

	videoApp := arcvideoplayback.NewMxPlayerApp(cr, tconn, kb, a, d, dataPath).(*arcvideoplayback.MxPlayerApp)
	if err := videoApp.Install(ctx); err != nil {
		return errors.Wrap(err, "failed to install MxPlayer app")
	}
	defer videoApp.Uninstall(closeCtx)
	cleanupFile, err := videoApp.CopyFileToFolder(ctx, VideoSrc)
	if err != nil {
		return errors.Wrap(err, "failed to copy video file to Downloads folder")
	}
	defer cleanupFile()

	const recordInterval = 5 * time.Second
	recorder := power.NewRecorder(ctx, recordInterval, outDir, testName)
	defer recorder.Close(closeCtx)
	if err := recorder.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed to cool down the device")
	}

	// Launch Element and arrange window.
	if err := socialApp.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to open Element")
	}
	defer socialApp.Close(closeCtx)

	if err := socialApp.SetUp(ctx); err != nil {
		return errors.Wrap(err, "failed to set up social app for testing")
	}
	defer socialApp.CleanUp(closeCtx)

	if err := arrangeWindow(ctx, tconn, apps.Element.ID, ash.WindowStatePrimarySnapped, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set Element window state and wait")
	}

	// Launch MxPlayer app and arrange window.
	if err := videoApp.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to launch MxPlayer app")
	}
	defer videoApp.Close(closeCtx)

	if err := uiauto.NamedCombine("set up MxPlayer",
		videoApp.DismissPrompts,
		videoApp.ChangeToResizable,
	)(ctx); err != nil {
		return err
	}
	if err := arrangeWindow(ctx, tconn, apps.MxPlayer.ID, ash.WindowStateNormal, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set MxPlayer window state and wait")
	}

	// Launch Chrome window and arrange window.
	// For TypeLacros, there is already an existing Chrome window, so skip launching.
	if bt != browser.TypeLacros {
		if _, err := uiHandler.LaunchChrome(ctx); err != nil {
			return errors.Wrap(err, "failed to launch Chrome window")
		}
	}

	if err := arrangeWindow(ctx, tconn, browserApp.ID, ash.WindowStateSecondarySnapped, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set browser window state and wait")
	}

	cleanupCtx := ctx
	// Given time to close apps.
	ctx, cancel = ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		shortCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		// Use a short timeout context to prevent getting stuck at "cuj.CloseAllTabs".
		if err := cuj.CloseAllTabs(shortCtx, bTconn, bt); err != nil {
			testing.ContextLog(ctx, "Failed to close all tabs: ", err)
			// Click the leave button if it exists.
			if err := prompts.ClearPotentialPrompts(tconn, time.Second, prompts.LeaveSitePrompt)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clear leave site prompt: ", err)
			}
		}
	}(cleanupCtx)

	defer func(ctx context.Context) {
		if retErr != nil {
			if err := d.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close the ARC UI device: ", err)
			}
			a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), func() bool { return retErr != nil })
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_tree")
		}
	}(cleanupCtx)

	if err := recorder.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start collecting power metrics")
	}

	conn, err := uiHandler.NewChromeTab(ctx, br, chrome.BlankURL, true)
	if err != nil {
		return errors.Wrap(err, "failed to create new chrome tab")
	}
	defer conn.Close()

	// Run at least 2 rounds as required, take 2 rounds to avoid test case taking too long.
	const loopCount = 2
	for i := 0; i < loopCount; i++ {
		if err := socialAppActivity(ctx, tconn, uiHandler, socialApp, socialAppTime); err != nil {
			return errors.Wrap(err, "failed to run social app procedure")
		}

		if err := videoAppActivity(ctx, uiHandler, i == 0 /*isFirstRun*/, videoApp, videoAppTime); err != nil {
			return errors.Wrap(err, "failed to run video app procedure")
		}

		if err := browserActivity(ctx, br, tconn, conn, uiHandler, browserApp, browserTime); err != nil {
			return errors.Wrap(err, "failed to run browser procedure")
		}
	}

	if err := recorder.Finish(ctx); err != nil {
		return errors.Wrap(err, "failed to finish collecting power metrics")
	}
	return nil

}

// arrangeWindow arranges the window for making all apps visible on the screen at the same time.
func arrangeWindow(ctx context.Context, tconn *chrome.TestConn, appID string, windowState ash.WindowStateType, tabletMode bool) error {
	// Since windows are always maximized in tablet mode,
	// arranging window is skipped here.
	if !tabletMode {
		window, err := ash.WaitForAppWindow(ctx, tconn, appID)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the app window")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, windowState); err != nil {
			return errors.Wrap(err, "failed to set video app window state and wait")
		}
	}

	return nil
}

// browserActivity defines test scenario of browser.
// Open a website, browse the page and wait 12 seconds.
// The total execution time is 6 minutes.
func browserActivity(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, conn *chrome.Conn, uiHandler cuj.UIActionHandler, browserApp apps.App, browserTime time.Duration) error {
	const (
		// chromeTabQuiescenceTimeout defines the maximum time duration to wait for a Chrome tab to achieve quiescence.
		chromeTabQuiescenceTimeout = time.Minute
		urlPrefix                  = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/v2_config/"
		configFile                 = "typical.json"
		redirectFile               = "redirect.html"
	)

	configURL := urlPrefix + configFile
	configJSON, err := utils.FetchFromURL(ctx, configURL)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch configuration from %s", configURL)
	}

	var config browsingConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return errors.Wrap(err, "failed to unmarshal configuration")
	}

	// Open webpage, swipe on page, and wait for 12 seconds, so the process takes roughly 2 minutes for 5 webpages.
	if err := uiHandler.SwitchToAppWindow(browserApp.Name)(ctx); err != nil {
		return errors.Wrap(err, "failed to switch to browser window")
	}

	const secsPerPage = 12 * time.Second
	siteList := config.URLData.Pages
	endTime := time.Now().Add(browserTime)
	for time.Now().Before(endTime) {
		for _, site := range siteList {
			startTime := time.Now()

			url := fmt.Sprintf("%s%s?ver=%s&dest=%s", urlPrefix, redirectFile, config.URLData.Version, site)
			if err := conn.Navigate(ctx, url); err != nil {
				return errors.Wrapf(err, "failed to navigate to %s", url)
			}

			// If the webpage's loading time times out, skip it and try the next one.
			if err := webutil.WaitForQuiescence(ctx, conn, chromeTabQuiescenceTimeout); err != nil {
				continue
			}

			if err := uiauto.NamedCombine("swipe on webpage and wait",
				uiHandler.SwipeDown(),
				// Sleep for 2 second in case there is lazy loading.
				uiauto.Sleep(2*time.Second),
				uiHandler.SwipeUp(),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to browse webpages")
			}

			browsingTime := startTime.Add(secsPerPage)
			// GoBigSleepLint: Sleep to measure power.
			if err := testing.Sleep(ctx, time.Until(browsingTime)); err != nil {
				return errors.Wrap(err, "failed to sleep")
			}

			// Since the number of sites may change, the total duration is uncertain.
			// Break the loop if the browsing timeout has exceeded.
			if time.Now().After(endTime) {
				break
			}
		}
	}

	return nil
}

// socialAppActivity defines test scenario of social app (Element in this case).
// Typing messages and rename chatroom name for a while.
func socialAppActivity(ctx context.Context, tconn *chrome.TestConn, uiHandler cuj.UIActionHandler, socialApp *socialapp.Element, socialAppTime time.Duration) error {
	// If can't find Element icon, try to relaunch it.
	if err := uiHandler.SwitchToAppWindow(apps.Element.Name)(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to switch to Element, try to launch it again")
		if err = socialApp.Launch(ctx); err != nil {
			return errors.Wrap(err, "failed to launch Element again")
		}
	}

	i := 1
	for endTime := time.Now().Add(socialAppTime); time.Now().Before(endTime); {
		if err := uiauto.NamedCombine(fmt.Sprintf("run social app test scenarios for %d times", i),
			socialApp.SendMessages,
			socialApp.RunExtraOperations,
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to run social app scenario")
		}
		i++
	}

	return nil
}

// videoAppActivity defines test scenario of video app (MxPlayer in this case).
// Play a video on MxPlayer for a while, then close the video.
func videoAppActivity(ctx context.Context, uiHandler cuj.UIActionHandler, isFirstRun bool, videoApp *arcvideoplayback.MxPlayerApp, videoPlayTime time.Duration) error {
	setLoopOn := func(ctx context.Context) error {
		// The loop settings of the video will be remembered. Only set the loop on
		// when it's the first time playing the video.
		if isFirstRun {
			return videoApp.SetLoopOn(ctx)
		}
		return nil
	}
	message := fmt.Sprintf("play a video for %v", videoPlayTime)
	return uiauto.NamedCombine(message,
		uiHandler.SwitchToAppWindow(apps.MxPlayer.Name),
		videoApp.OpenAndPlayVideo(videoName),
		setLoopOn,
		uiauto.Sleep(videoPlayTime),
		videoApp.CloseVideo(videoName),
	)(ctx)
}
