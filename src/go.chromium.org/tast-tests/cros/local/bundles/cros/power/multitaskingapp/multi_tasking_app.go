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
	"time"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/arcvideoplayback"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/socialapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/element"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
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
	WebSource     cuj.WebSourceType
	TestName      string
	OutDir        string
	ElementAPKURL string
	DataPath      func(string) string
	Discharge     bool
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
	A         *arc.ARC
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
	VideoSrc      = "multitaskingapp/vp9_720_60fps.webm"
	videoFileName = "vp9_720_60fps.webm"
)

// Run runs the MultitaskingApp test.
func Run(ctx context.Context, resources *TestResources, params *TestParams) (retErr error) {

	var (
		outDir        = params.OutDir
		testName      = params.TestName
		elementAPKURL = params.ElementAPKURL
		dataPath      = params.DataPath
		discharge     = params.Discharge
		browserTime   = params.BrowserTime
		socialAppTime = params.SocialAppTime
		videoAppTime  = params.VideoAppTime
		cr            = resources.Cr
		tconn         = resources.Tconn
		a             = resources.A
		kb            = resources.Kb
		uiHandler     = resources.UIHandler
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Minimize the zoom factor to ensure all the objects in the apps can be shown on the screen.
	revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to set the zoom factor of the primary display to minimum")
	}
	defer revertZoom(cleanupCtx, tconn)

	browserApp, err := apps.ChromeOrChromium(ctx, tconn)
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
	}(cleanupCtx)

	socialApp := socialapp.NewElement(tconn, kb, a, d, cr, elementAPKURL)
	if err := socialApp.Install(ctx); err != nil {
		return errors.Wrap(err, "failed to install social app")
	}
	defer socialApp.Uninstall(cleanupCtx)

	if err := arc.DisableAppNotifications(ctx, a, element.ElementPackage); err != nil {
		return errors.Wrap(err, "failed to disable social app notifications")
	}

	videoApp := arcvideoplayback.NewExoPlayerApp(cr, tconn, kb, a, d, dataPath).(*arcvideoplayback.ExoPlayerApp)
	if err := videoApp.Install(ctx); err != nil {
		return errors.Wrap(err, "failed to install video app")
	}
	defer videoApp.Uninstall(cleanupCtx)
	cleanupFile, err := videoApp.CopyFileToFolder(ctx, VideoSrc)
	if err != nil {
		return errors.Wrap(err, "failed to copy video file to Downloads folder")
	}
	defer cleanupFile()

	const recordInterval = 5 * time.Second
	recorder := power.NewRecorder(ctx, recordInterval, outDir, testName, power.DischargeWatchdogOption(discharge))
	defer recorder.Close(cleanupCtx)
	if err := recorder.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed to cool down the device")
	}

	// Launch Element and arrange window.
	if err := socialApp.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to open Element")
	}
	defer socialApp.Close(cleanupCtx)

	isSocialAppSetup := false
	defer func(ctx context.Context) {
		// Only dump the faillog of the setup steps here.
		// The faillog of the main test procedure is separately dumped.
		if !isSocialAppSetup {
			// Close the ARC UI device before dumping ARC UI Hierarchy.
			// Otherwise uiautomator might exist with errors.
			if err := d.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close the ARC UI device: ", err)
			}
			a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), func() bool { return retErr != nil })
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "setup_dump")
		} else {
			if !d.Alive(ctx) {
				if newDevice, err := a.NewUIDevice(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to create new UI device for social app cleanup: ", err)
				} else {
					socialApp.SetUIDevice(newDevice)
					defer newDevice.Close(ctx)
				}
			}

			if err := socialApp.CleanUp(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clean up social app: ", err)
			}
		}
	}(cleanupCtx)

	if err := socialApp.SetUp(ctx); err != nil {
		return errors.Wrap(err, "failed to set up social app for testing")
	}
	isSocialAppSetup = true

	if err := arrangeWindow(ctx, tconn, apps.Element.ID, ash.WindowStatePrimarySnapped, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set Element window state and wait")
	}

	if err := videoApp.Launch(ctx); err != nil {
		return errors.Wrap(err, "failed to launch video app")
	}
	defer videoApp.Close(cleanupCtx)

	if err := arrangeWindow(ctx, tconn, videoApp.ID(), ash.WindowStateNormal, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set video app window state and wait")
	}

	// Launch Chrome window and arrange window.
	if _, err := uiHandler.LaunchChrome(ctx); err != nil {
		return errors.Wrap(err, "failed to launch Chrome window")
	}

	if err := arrangeWindow(ctx, tconn, browserApp.ID, ash.WindowStateSecondarySnapped, params.TabletMode); err != nil {
		return errors.Wrap(err, "failed to set browser window state and wait")
	}

	closeCtx := ctx
	// Given time to close apps.
	ctx, cancel = ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		shortCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		// Use a short timeout context to prevent getting stuck at "CloseAllTabs".
		if err := browser.CloseAllTabs(shortCtx, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to close all tabs: ", err)
			// Click the leave button if it exists.
			if err := prompts.ClearPotentialPrompts(tconn, time.Second, prompts.LeaveSitePrompt)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clear leave site prompt: ", err)
			}
		}
	}(closeCtx)

	defer func(ctx context.Context) {
		if retErr != nil {
			if err := d.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close the ARC UI device: ", err)
			}
			a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), func() bool { return retErr != nil })
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_tree")
		}
	}(closeCtx)

	if err := recorder.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start collecting power metrics")
	}

	conn, err := uiHandler.NewChromeTab(ctx, chrome.BlankURL, true)
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

		if err := browserActivity(ctx, tconn, conn, uiHandler, browserApp, browserTime); err != nil {
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
func browserActivity(ctx context.Context, tconn *chrome.TestConn, conn *chrome.Conn, uiHandler cuj.UIActionHandler, browserApp apps.App, browserTime time.Duration) error {
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
				prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.BlockLocalNetworkPrompt),
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
	if err := uiauto.Combine("ensure room access",
		// If can't find Element icon, try to relaunch it.
		uiauto.IfFailThen(uiHandler.SwitchToAppWindow(apps.Element.Name), socialApp.Launch),
		socialApp.EnsureInRoom(),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure room access")
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

// videoAppActivity defines test scenario of video app.
// Play a video on for a while, then close the video.
func videoAppActivity(ctx context.Context, uiHandler cuj.UIActionHandler, isFirstRun bool, videoApp *arcvideoplayback.ExoPlayerApp, videoPlayTime time.Duration) error {
	dismissPrompt := func(ctx context.Context) error {
		// The permission prompts only need to be dismissed once.
		if isFirstRun {
			return videoApp.DismissPrompts(ctx)
		}
		return nil
	}
	message := fmt.Sprintf("play a video for %v", videoPlayTime)
	return uiauto.NamedCombine(message,
		uiHandler.SwitchToAppWindow(videoApp.Name()),
		videoApp.PlayVideoInLoop(videoFileName),
		dismissPrompt,
		videoApp.EnsurePlaying,
		uiauto.Sleep(videoPlayTime),
		videoApp.Pause,
		videoApp.CloseVideo,
	)(ctx)
}
