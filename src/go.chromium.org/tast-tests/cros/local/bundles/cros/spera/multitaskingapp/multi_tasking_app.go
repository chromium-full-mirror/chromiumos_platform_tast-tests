// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package multitaskingapp contains the test code for spera.MultiTaskingApp test.
package multitaskingapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	youtubeMusicAppName = "YouTube Music"

	initialVolume   = 60
	mediumUITimeout = 30 * time.Second // Used for situations where UI response are slower.
)

// TestParams holds the parameters to run the test main logic.
type TestParams struct {
	Tier            cuj.Tier
	BrowserType     browser.Type
	WebSource       cuj.WebSourceType
	OutDir          string
	TraceConfigPath string
	TabletMode      bool
}

var (
	googleURLList   = []string{cuj.GoogleFinanceURL, cuj.GoogleWorkspaceURL, cuj.GooglePolicyURL, cuj.GoogleNonprofitsURL}
	externalURLList = []string{cuj.HuluURL, cuj.CnnURL, cuj.WikipediaURL, cuj.RedditURL}

	urlListMap = map[cuj.WebSourceType][]string{
		cuj.ExternalWebSource: externalURLList,
		cuj.GoogleWebSource:   googleURLList,
	}
)

type runResources struct {
	kb         *input.KeyboardEventWriter
	topRow     *input.TopRowLayout
	ui         *uiauto.Context
	vh         *audio.Helper
	uiHandler  cuj.UIActionHandler
	browserApp apps.App
	bTconn     *chrome.TestConn
}

// Run runs the MultitaskingApp test.
func Run(ctx context.Context, cr *chrome.Chrome, params *TestParams) error {
	bt := params.BrowserType
	tabletMode := params.TabletMode
	traceConfigPath := params.TraceConfigPath
	outDir := params.OutDir

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}
	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)

	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}

	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create the volumeHelper")
	}
	originalVolume, err := vh.GetVolume(ctx)
	defer vh.SetVolume(cleanupCtx, originalVolume)

	isMuted, err := vh.IsMuted(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the mute status")
	}
	if isMuted {
		testing.ContextLog(ctx, "Press 'VolumeUp' to unmute")
		if err := kb.Accel(ctx, topRow.VolumeUp); err != nil {
			return errors.Wrap(err, "failed to press volumeUp key to unmute")
		}
		defer kb.Accel(cleanupCtx, topRow.VolumeMute)
	}

	// uiHandler will be assigned with different instances for clamshell and tablet mode.
	var uiHandler cuj.UIActionHandler
	if tabletMode {
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create tablet action handler")
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create clamshell action handler")
		}
	}
	defer uiHandler.Close(ctx)

	testing.ContextLog(ctx, "Start to get browser start time")
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode, bt)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}
	if l != nil {
		defer l.Close(ctx)
	}
	br := cr.Browser()
	if l != nil {
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to create Test API connection for %v browser", bt)
	}
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "could not find the Chrome app")
	}

	// Give 10 seconds to set initial settings. It is critical to ensure
	// cleanupSetting can be executed with a valid context so it has its
	// own cleanup context from other cleanup functions. This is to avoid
	// other cleanup functions executed earlier to use up the context time.
	cleanupSettingsCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupSetting, err := cuj.InitializeSetting(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to set initial settings")
	}
	defer cleanupSetting(cleanupSettingsCtx)

	// Set up the cuj.Recorder: this test will measure the combinations of
	// animation smoothness for window-cycles (alt-tab selection), launcher,
	// and overview.
	// Shorten the context to cleanup recorder.
	cleanUpRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(cleanUpRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(bt, tconn, bTconn, recorder); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		if traceConfigPath != "" {
			// Start tracing now.
			if err := recorder.StartTracing(ctx, outDir, traceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			defer recorder.StopTracing(ctx)
		}

		// Given time to close chrome.
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 20*time.Second)
		defer cancel()

		defer func(ctx context.Context) {
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_tree")

			shortCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			closeFunc := func(ctx context.Context) error {
				// Use a short timeout context to prevent getting stuck at "cuj.CloseAllTabs".
				if err := cuj.CloseAllTabs(shortCtx, bTconn, bt); err != nil {
					testing.ContextLog(ctx, "Failed to close all tabs: ", err)
					// When closing the "Youtube Music" website, the popup "Leave site?" might appear.
					// Click the leave button if it exists.
					if err := prompts.ClearPotentialPrompts(tconn, time.Second, prompts.LeaveSitePrompt)(ctx); err != nil {
						testing.ContextLog(ctx, "Failed to clear leave site prompt: ", err)
					}
				}
				return nil
			}
			if err := cuj.RunAndWaitLCPHistograms(ctx, bTconn, closeFunc); err != nil {
				testing.ContextLog(ctx, "Failed to run and wait for LCP histograms to update: ", err)
			}
		}(cleanupCtx)

		resources := &runResources{
			kb:         kb,
			topRow:     topRow,
			ui:         ui,
			vh:         vh,
			uiHandler:  uiHandler,
			browserApp: browserApp,
			bTconn:     bTconn,
		}

		if err := openAndSwitchTabs(ctx, br, tconn, params, resources); err != nil {
			return errors.Wrap(err, "failed to open and switch chrome tabs")
		}
		if err := switchWindows(ctx, tconn, resources, tabletMode); err != nil {
			return errors.Wrap(err, "failed to switch windows")
		}
		if err := takePhotoAndVideo(ctx, cr, outDir); err != nil {
			return errors.Wrap(err, "failed to take photo and video")
		}
		if err := cuj.GenerateADF(ctx, tconn, tabletMode); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to run the multi tasking app")
	}

	pv := perf.NewValues()

	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := recorder.Record(recordCtx, pv); err != nil {
		return errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(recordCtx); err != nil {
		testing.ContextLog(recordCtx, "Failed to save trace files: ", err)
	}
	if err = pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}

	return nil
}

func openAndSwitchTabs(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, params *TestParams, resources *runResources) error {
	uiHandler := resources.uiHandler
	browserApp := resources.browserApp
	vh := resources.vh
	kb := resources.kb
	topRow := resources.topRow
	bTconn := resources.bTconn

	webSource := params.WebSource
	if _, ok := urlListMap[webSource]; !ok {
		return errors.Errorf("unknown web source: %v", webSource)
	}

	urlList := urlListMap[webSource]
	// Essential tier test scenario: 2 browser windows open with 5 tabs each.
	// Advanced tier test scenario: Same as essential but click through 20 tabs (4 windows x 5 tabs):
	// 1. The first and second window URL lists are the same as essential.
	// 2. The fourth window URL list is the same as the third one.
	firstWindowURLList := []string{cuj.GmailURL, cuj.GoogleCalendarURL, cuj.YoutubeMusicURL, urlList[0], cuj.GoogleNewsURL}
	secondWindowURLList := []string{cuj.GoogleNewsURL, urlList[1], urlList[2], cuj.GoogleNewsURL, urlList[1]}
	thirdWindowURLList := []string{cuj.GoogleNewsURL, urlList[1], urlList[2], urlList[3], urlList[1]}
	fourthWindowURLList := thirdWindowURLList

	pageList := [][]string{firstWindowURLList, secondWindowURLList}
	if params.Tier == cuj.Advanced {
		pageList = append(pageList, thirdWindowURLList, fourthWindowURLList)
	}

	openBrowserWithTabs := func(urlList []string) error {
		for idx, url := range urlList {
			conn, err := uiHandler.NewChromeTab(ctx, br, url, idx == 0)
			if err != nil {
				return errors.Wrapf(err, "failed to open %s", url)
			}
			// We don't need to keep the connection, so close it before leaving this function.
			defer conn.Close()

			timeout := time.Minute
			if err := webutil.WaitForQuiescence(ctx, conn, timeout); err != nil {
				// It has been seen that content sites such as CNN (https://edition.cnn.com/) sometimes can take
				// minutes to reach quiescence on DUTs. When this occurred, it can be seen from screenshots that
				// the UI has actually loaded but background tasks prevented the site to reach quiescence. Therefore,
				// logic is added here to check whether the site has loaded. If the site has loaded, i.e., the site
				// readyState is not "loading", no error will be returned here.
				if err := conn.WaitForExpr(ctx, `document.readyState === "interactive" || document.readyState === "complete"`); err == nil {
					testing.ContextLogf(ctx, "%s could not reach quiescence, but document state has passed loading", url)
					continue
				}
				return errors.Wrapf(err, "failed to wait for page to finish loading within %v [%s]", timeout, url)
			}

			if url == cuj.GoogleWorkspaceURL {
				if err := cuj.WaitForPDFVideoHistogram(ctx, bTconn); err != nil {
					testing.ContextLog(ctx, "Failed to wait for PDF histogram: ", err)
				}
			}

			if url == cuj.YoutubeMusicURL {
				if err := playYoutubeMusic(ctx, tconn); err != nil {
					return errors.Wrap(err, "failed to play Youtube Music")
				}
				if err := cuj.GenerateEventLatency(ctx, tconn, youtubeMusicAppName); err != nil {
					return errors.Wrap(err, "failed to generate event latency histograms")
				}
			}
		}
		return nil
	}

	// switchTabsAndChangeVolume changes the volume after switching tabs.
	switchTabsAndChangeVolume := func(ctx context.Context, browserWinIdx int, pages []string) error {
		if err := vh.SetVolume(ctx, initialVolume); err != nil {
			return errors.Wrapf(err, "failed to set volume to %d percent", initialVolume)
		}

		for tabIdx := range pages {
			testing.ContextLog(ctx, "Switching Chrome tab")
			if err := uiHandler.SwitchToChromeTabByIndex(tabIdx)(ctx); err != nil {
				return err
			}
			testing.ContextLog(ctx, "Volume up")
			if err := vh.VerifyVolumeChanged(ctx, func() error {
				return kb.Accel(ctx, topRow.VolumeUp)
			}); err != nil {
				return errors.Wrap(err, `volume not changed after press "VolumeUp"`)
			}

			// GoBigSleepLint - After applying new volume, stay on the tab with the volume for 2 seconds before applying next one.
			if err := testing.Sleep(ctx, 2*time.Second); err != nil {
				return errors.Wrap(err, "failed to sleep")
			}
		}
		return nil
	}

	switchAllBrowserTabs := func(ctx context.Context) error {
		testing.ContextLog(ctx, "Start to switch all browser tabs")
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to obtain the window list")
		}

		browserWinIdx := -1
		for range ws {
			// Cycle through all windows, find out the browser window, and do tab switch.

			// Switch to the least recent window to start from the window we opened first.
			testing.ContextLog(ctx, "Switching window by overview")
			if err := uiHandler.SwitchToLRUWindow(cuj.SwitchWindowThroughOverview)(ctx); err != nil {
				return errors.Wrap(err, "failed to switch windows through overview")
			}

			w, err := ash.GetActiveWindow(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to get active window")
			}
			if w.WindowType != ash.WindowTypeBrowser && w.WindowType != ash.WindowTypeLacros {
				continue
			}
			browserWinIdx++
			if err := switchTabsAndChangeVolume(ctx, browserWinIdx, pageList[browserWinIdx]); err != nil {
				return errors.Wrap(err, "failed to switch tabs")
			}
		}
		return nil
	}

	if browserApp.ID == apps.Lacros.ID {
		activeWindow, err := ash.GetActiveWindow(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get the active window")
		}
		if activeWindow.WindowType != ash.WindowTypeLacros {
			if err := uiHandler.SwitchToAppWindow(browserApp.Name)(ctx); err != nil {
				return errors.Wrap(err, "failed to switch to lacros window")
			}
		}
	}
	for _, list := range pageList {
		if err := openBrowserWithTabs(list); err != nil {
			return errors.Wrap(err, "failed to open browser with tabs")
		}
	}
	if err := switchAllBrowserTabs(ctx); err != nil {
		return errors.Wrap(err, "failed to switch all browser tabs")
	}

	return nil
}

func switchWindows(ctx context.Context, tconn *chrome.TestConn, resources *runResources, tabletMode bool) error {
	uiHandler := resources.uiHandler
	vh := resources.vh
	kb := resources.kb
	topRow := resources.topRow
	browserApp := resources.browserApp
	// subtest defines the detail of the window switch test procedure. It could be different for clamshell and tablet mode.
	type subtest struct {
		name string
		desc string
		// switchWindowFunc is the function used to do window switch.
		// ws is all the application windows in the system.
		// i is the index of the target window switching to.
		switchWindowFunc func(ctx context.Context, ws []*ash.Window, i int) error
	}
	// switchWindowByOverviewTest is the common switch window test for clamshell and tablet.
	switchWindowByOverviewTest := subtest{
		"overview",
		"Switching the focused window through the overview mode",
		func(ctx context.Context, ws []*ash.Window, i int) error {
			testing.ContextLog(ctx, "Switching window by overview")
			return uiHandler.SwitchToLRUWindow(cuj.SwitchWindowThroughOverview)(ctx)
		},
	}
	// switchWindowTests holds a serial of window switch tests. It has different subtest for clamshell and tablet mode.
	switchWindowTests := []subtest{switchWindowByOverviewTest}
	if tabletMode {
		switchWindowByHotseatTest := subtest{
			"hotseat",
			"Switching the focused window through clicking the hotseat",
			func(ctx context.Context, ws []*ash.Window, i int) error {
				wIdx := 0 // The index of target window within same app.
				for idx := 0; idx < i; idx++ {
					// Count the windows of same type before the target window.
					if ws[idx].WindowType == ws[i].WindowType {
						wIdx++
					}
				}
				winName := browserApp.Name

				testing.ContextLogf(ctx, "Switching window to %q", ws[i].Title)
				return uiHandler.SwitchToAppWindowByIndex(winName, wIdx)(ctx)
			},
		}
		switchWindowTests = append(switchWindowTests, switchWindowByHotseatTest)
	} else {
		switchWindowByKeyboardTest := subtest{
			"alt-tab",
			"Switching the focused window through Alt-Tab",
			func(ctx context.Context, ws []*ash.Window, i int) error {
				return uiHandler.SwitchToLRUWindow(cuj.SwitchWindowThroughKeyEvent)(ctx)
			},
		}
		switchWindowTests = append(switchWindowTests, switchWindowByKeyboardTest)
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get window list")
	}

	for _, subtest := range switchWindowTests {
		testing.ContextLog(ctx, subtest.desc)
		if err := vh.SetVolume(ctx, initialVolume); err != nil {
			return errors.Wrapf(err, "failed to set volume to %v percents", initialVolume)
		}
		testing.ContextLog(ctx, "Volume up")
		if err := vh.VerifyVolumeChanged(ctx, func() error {
			return kb.Accel(ctx, topRow.VolumeUp)
		}); err != nil {
			return errors.Wrap(err, `volume not changed after press "VolumeUp"`)
		}

		for i := range ws {
			// Switch between windows by calling the switch window function.
			if err := subtest.switchWindowFunc(ctx, ws, i); err != nil {
				return errors.Wrap(err, "failed to switch window")
			}
		}
	}

	return nil
}

func takePhotoAndVideo(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	testing.ContextLog(ctx, "Take photo and video")

	tb, err := testutil.NewTestBridgeWithoutTestConfig(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		return errors.Wrap(err, "failed to construct test bridge")
	}
	defer tb.TearDown(ctx)

	if err := cca.ClearSavedDir(ctx, cr); err != nil {
		return errors.Wrap(err, "failed to clear saved directory")
	}

	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		return errors.Wrap(err, "failed to open CCA")
	}
	defer app.Close(ctx)

	_, err = app.TakeSinglePhoto(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to take single photo")
	}

	testing.ContextLog(ctx, "Switch to video mode")
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	testing.ContextLog(ctx, "Start to record video")
	if _, err := app.RecordVideo(ctx, cca.TimerOn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to record video")
	}

	return nil
}

func playYoutubeMusic(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	shuffleButton := nodewith.Name("Shuffle").Role(role.Button)
	pauseButton := nodewith.Name("Pause").Role(role.Button).First()
	reviewIconUpdateWindow := nodewith.Name("Review icon update").Role(role.Window)
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(reviewIconUpdateWindow)
	dismissReviewIconUpdateIfPresent := uiauto.IfSuccessThen(
		ui.WithTimeout(3*time.Second).WaitUntilExists(reviewIconUpdateWindow),
		uiauto.NamedAction("close 'Review icon update' dialog", ui.LeftClick(okButton)))
	waitPauseButton := uiauto.NamedCombine("wait pause button",
		dismissReviewIconUpdateIfPresent,
		ui.WaitUntilExists(pauseButton),
	)
	// Sometimes closing the dialog doesn't work, so add retry here.
	return uiauto.NamedAction("play youtube music",
		ui.WithTimeout(mediumUITimeout).DoDefaultUntil(shuffleButton, waitPauseButton))(ctx)
}
