// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package youtube contains the test code for VideoCUJ.
package youtube

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// YoutubeWeb indicates to test against Youtube web.
	YoutubeWeb = "YoutubeWeb"
	// YoutubeApp indicates to test against Youtube app.
	YoutubeApp = "YoutubeApp"
	// YoutubeWindowTitle indicates the title of the youtube web and app window.
	YoutubeWindowTitle = "YouTube"
)

// TestResources holds the cuj test resources passed in from main test case.
type TestResources struct {
	Cr        *chrome.Chrome
	Tconn     *chrome.TestConn
	A         *arc.ARC
	Kb        *input.KeyboardEventWriter
	UIHandler cuj.UIActionHandler
}

// TestParams holds the cuj test parameters passed in from main test case.
type TestParams struct {
	OutDir          string
	App             string
	TabletMode      bool
	Tier            cuj.Tier
	ExtendedDisplay bool
	CheckPIP        bool
	TraceConfigPath string
	YoutubeApkURL   string
}

// VideoApp declares video operation.
type VideoApp interface {
	// Install installs the Youtube app with apk.
	Install(ctx context.Context) error
	// OpenAndPlayVideo opens a video.
	OpenAndPlayVideo(video VideoSrc) uiauto.Action
	// EnterFullScreen switches video to full screen.
	EnterFullScreen(ctx context.Context) error
	// ExitFullScreen exits Youtube video from full screen.
	ExitFullScreen(ctx context.Context) error
	// PauseAndPlayVideo verifies video playback.
	PauseAndPlayVideo(ctx context.Context) error
	// IsPlaying verifies video is playing.
	IsPlaying() uiauto.Action
	// Close closes the resources related to video.
	Close(ctx context.Context)
}

// VideoSrc struct defines video src for testing.
type VideoSrc struct {
	URL     string
	Title   string
	Quality Quality
}

var basicVideoSrc = []VideoSrc{
	{
		cuj.YoutubeGoogleTVVideoURL,
		"Chris Paul | Watch With Me | Google TV",
		Quality1080P,
	},
	{
		cuj.YoutubeDeveloperKeynoteVideoURL,
		"Developer Keynote (Google I/O '21) - American Sign Language",
		Quality720P60,
	},
	{
		cuj.YoutubeStadiaGDCVideoURL,
		"Stadia GDC 2019 Gaming Announcement",
		Quality1080P60,
	},
}

var premiumVideoSrc = []VideoSrc{
	{
		cuj.YoutubeStadiaGDCVideoURL,
		"Stadia GDC 2019 Gaming Announcement",
		Quality2160P60,
	},
}

// Run runs the VideoCUJ test.
func Run(ctx context.Context, resources TestResources, param TestParams) error {
	var (
		cr              = resources.Cr
		tconn           = resources.Tconn
		a               = resources.A
		kb              = resources.Kb
		uiHandler       = resources.UIHandler
		outDir          = param.OutDir
		appName         = param.App
		tabletMode      = param.TabletMode
		tier            = param.Tier
		extendedDisplay = param.ExtendedDisplay
		traceConfigPath = param.TraceConfigPath
		youtubeApkURL   = param.YoutubeApkURL
	)

	testing.ContextLogf(ctx, "Run app appName: %s tabletMode: %t, extendedDisplay: %t", appName, tabletMode, extendedDisplay)

	tabChecker, err := cuj.NewTabCrashChecker(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to create TabCrashChecker")
	}

	ui := uiauto.New(tconn)

	// Give 10 seconds to set initial settings. It is critical to ensure
	// cleanupSetting can be executed with a valid context so it has its
	// own cleanup context from other cleanup functions. This is to avoid
	// other cleanup functions executed earlier to use up the context time.
	cleanupSettingsCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupSetting, err := cuj.InitializeSetting(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to set initial settings")
	}
	defer cleanupSetting(cleanupSettingsCtx)

	testing.ContextLog(ctx, "Start to get browser start time")
	browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}

	videoSources := basicVideoSrc
	if tier == cuj.Premium || tier == cuj.Advanced {
		videoSources = premiumVideoSrc
	}

	// Give 5 seconds to clean up device objects connected to UI Automator server resources.
	cleanupDeviceCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create new ARC device")
	}
	defer func(ctx context.Context) {
		if d.Alive(ctx) {
			testing.ContextLog(ctx, "UI device is still alive")
			d.Close(ctx)
		}
	}(cleanupDeviceCtx)

	// Give 5 seconds to cleanup recorder.
	cleanupRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, a, options)
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(cleanupRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(recorder); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}

	var videoApp VideoApp
	switch appName {
	case YoutubeWeb:
		videoApp = NewYtWeb(tconn, kb, extendedDisplay, ui, uiHandler)
	case YoutubeApp:
		videoApp, err = NewYtApp(ctx, cr, kb, a, d, outDir, youtubeApkURL)
		if err != nil {
			return err
		}
		if err := videoApp.Install(ctx); err != nil {
			return errors.Wrap(err, "failed to install Youtube app")
		}
	}

	traceRecorder := false
	run := func(ctx context.Context, videoSource VideoSrc) (retErr error) {
		defer func(ctx context.Context) {
			// Make sure to close the arc UI device before calling the function. Otherwise uiautomator might have errors.
			if appName == YoutubeApp && retErr != nil {
				if err := d.Close(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to close ARC UI device: ", err)
				}
				a.DumpUIHierarchyOnError(ctx, filepath.Join(outDir, "arc"), func() bool { return retErr != nil })
			}
			if appName == YoutubeApp {
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")
				videoApp.Close(ctx)
			}
			closeFunc := func(ctx context.Context) error {
				if err := browser.CloseAllTabs(ctx, tconn); err != nil {
					testing.ContextLog(ctx, "Failed to close all tabs: ", err)
				}
				return nil
			}
			if err := cuj.RunAndWaitLCPHistograms(ctx, tconn, closeFunc); err != nil {
				testing.ContextLog(ctx, "Failed to run and wait for LCP histograms to update: ", err)
			}
			// Close the currently playing video and restart the new one.
			if appName == YoutubeWeb {
				// Before closing the youtube site outside the recorder, dump the UI tree to capture a screenshot.
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")
				if err := browser.CloseAllTabs(ctx, tconn); err != nil {
					testing.ContextLog(ctx, "Failed to close all tabs: ", err)
				}
			}
		}(ctx)
		ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
		defer cancel()

		// Record tracing in the first iteration.
		if traceConfigPath != "" && !traceRecorder {
			if err := recorder.StartTracing(ctx, outDir, traceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			traceRecorder = true
			defer recorder.StopTracing(ctx)
		}

		if err := videoScenario(ctx, resources, param, videoApp, videoSource, tabChecker); err != nil {
			return errors.Wrap(err, "failed to run video test")
		}
		if err := cuj.GenerateADF(ctx, tconn, tabletMode); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}
		return nil
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		for _, videoSource := range videoSources {
			if err := run(ctx, videoSource); err != nil {
				return errors.Wrapf(err, "failed to run %q video playback", appName)
			}
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to run the recorder task")
	}

	pv := perf.NewValues()

	// We'll collect Browser.StartTime for both YouTube-Web and YouTube-App
	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, float64(browserStartTime.Milliseconds()))

	if appName == YoutubeApp {
		pv.Set(perf.Metric{
			Name:      "Apps.StartTime",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, float64(appStartTime.Milliseconds()))
	}

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := recorder.Record(recordCtx, pv); err != nil {
		return errors.Wrap(err, "failed to record the performance metrics")
	}
	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save performance metrics")
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := recorder.SaveTraceFiles(recordCtx); err != nil {
		testing.ContextLog(recordCtx, "Failed to save trace files: ", err)
	}
	return nil
}

func videoScenario(ctx context.Context, resources TestResources, param TestParams,
	videoApp VideoApp, videoSrc VideoSrc, tabChecker *cuj.TabCrashChecker) error {

	var (
		appName         = param.App
		extendedDisplay = param.ExtendedDisplay
		checkPIP        = param.CheckPIP
		uiHandler       = resources.UIHandler
		tconn           = resources.Tconn
		kb              = resources.Kb
	)

	ui := uiauto.New(tconn)
	openGoogleHelp := func(ctx context.Context) error {
		conn, err := uiHandler.NewChromeTab(ctx, cuj.GoogleHelpChromeURL, true)
		if err != nil {
			return errors.Wrap(err, "failed to open Google Help")
		}
		defer conn.Close()
		if err := webutil.WaitForQuiescence(ctx, conn, 2*time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for Google Help to finish loading")
		}
		if err := cuj.GenerateEventLatency(ctx, tconn, "Google Help"); err != nil {
			return errors.Wrap(err, "failed to generate event latency histograms")
		}
		googleHelpHeading := nodewith.Name("How can we help you?").Role(role.Heading)
		if err := ui.LeftClick(googleHelpHeading)(ctx); err != nil {
			return err
		}
		if err := cuj.GeneratePDF(ctx, tconn, kb); err != nil {
			testing.ContextLog(ctx, "Failed to generate PDF histogram: ", err)
		}
		return nil
	}

	if err := videoApp.OpenAndPlayVideo(videoSrc)(ctx); err != nil {
		return errors.Wrapf(err, "failed to open %s", appName)
	}

	// Play video at full screen.
	if err := videoApp.EnterFullScreen(ctx); err != nil {
		return errors.Wrap(err, "failed to play video in fullscreen")
	}
	// After entering full screen, it must be in playback state.
	// This will make sure to switch to pip mode.
	if err := uiauto.Retry(3, videoApp.IsPlaying())(ctx); err != nil {
		return errors.Wrap(err, "failed to verify video is playing")
	}

	testing.ContextLog(ctx, "Open Google Help")
	if err := openGoogleHelp(ctx); err != nil {
		return errors.Wrap(err, "failed to open Google Help")
	}

	// YouTube sometimes pops up a prompt to notice users how to operate YouTube
	// if there're new features. Dismiss prompt if it exist.
	gotItPrompt := nodewith.Name("Got it").Role(role.Button)
	if err := uiauto.IfSuccessThen(
		ui.WaitUntilExists(gotItPrompt),
		uiHandler.ClickUntil(
			gotItPrompt,
			ui.WithTimeout(2*time.Second).WaitUntilGone(gotItPrompt),
		),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click 'Got it' prompt")
	}

	ytApp, ok := videoApp.(*YtApp)
	// Only do PiP testing for YT APP and when logged in as premium user.
	if ok && checkPIP && ytApp.isPremiumAccount() {
		if err := ytApp.checkYoutubeAppPIP(ctx); err != nil {
			return errors.Wrap(err, "youtube app smaller video preview window is not shown")
		}
	}

	// Switch back to video playing.
	if appName == YoutubeApp {
		if err := uiHandler.SwitchToAppWindow("YouTube")(ctx); err != nil {
			return errors.Wrap(err, "failed to switch to YouTube app")
		}
	} else {
		if err := uiHandler.SwitchWindow()(ctx); err != nil {
			return errors.Wrap(err, "failed to switch back to video playing")
		}
	}

	// Pause and resume video playback.
	if err := videoApp.PauseAndPlayVideo(ctx); err != nil {
		return errors.Wrap(err, "failed to pause and play video")
	}

	if extendedDisplay {
		if err := moveGmailWindow(ctx, resources); err != nil {
			return errors.Wrap(err, "failed to move Gmail window between main display and extended display")
		}
		if appName == YoutubeWeb {
			if err := moveYTWebWindow(ctx, resources); err != nil {
				return errors.Wrap(err, "failed to move YT Web window to internal display")
			}
		}
	}

	if err := videoApp.ExitFullScreen(ctx); err != nil {
		return errors.Wrap(err, "failed to exit full screen")
	}

	// Before recording the metrics, check if there is any tab crashed.
	if err := tabChecker.Check(ctx); err != nil {
		return errors.Wrap(err, "tab renderer crashed")
	}
	return nil
}

func waitWindowStateFullscreen(tconn *chrome.TestConn, winTitle string) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Check if the window is in fullscreen state")
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return strings.Contains(w.Title, winTitle) && w.State == ash.WindowStateFullscreen
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for fullscreen")
		}
		return nil
	}
}

func waitWindowStateExitFullscreen(tconn *chrome.TestConn, winTitle string) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Check if the window is in fullscreen state")
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			return strings.Contains(w.Title, winTitle) && w.State != ash.WindowStateFullscreen
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for exit from fullscreen")
		}
		return nil
	}
}

func getFirstWindowID(ctx context.Context, tconn *chrome.TestConn) (int, error) {
	all, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get all windows")
	}
	if len(all) != 1 {
		for _, win := range all {
			testing.ContextLogf(ctx, "%+v", *win)
		}
		testing.ContextLogf(ctx, "Expect 1 window, got %d", len(all))
	}
	return all[0].ID, nil
}

// moveGmailWindow switches Gmail to the extended display and switches back to internal display.
func moveGmailWindow(ctx context.Context, testRes TestResources) error {
	return uiauto.Combine("switch to gmail and move it between two displays",
		testRes.UIHandler.SwitchWindow(),
		cuj.SwitchWindowToDisplay(ctx, testRes.Tconn, testRes.Kb, true),  // Move to external display.
		uiauto.Sleep(2*time.Second),                                      // Keep the window in external display for 2 second.
		cuj.SwitchWindowToDisplay(ctx, testRes.Tconn, testRes.Kb, false), // Move to internal display.
	)(ctx)
}

// moveYTWebWindow switches Youtube Web to the internal display.
func moveYTWebWindow(ctx context.Context, testRes TestResources) error {
	return uiauto.Combine("switch to YT Web and move it to internal display",
		testRes.UIHandler.SwitchWindow(),
		cuj.SwitchWindowToDisplay(ctx, testRes.Tconn, testRes.Kb, false), // Move to internal display.
	)(ctx)
}
