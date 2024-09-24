// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package deskscuj contains helper util and test code for DesksCUJ.
package deskscuj

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"github.com/mafredri/cdp/protocol/target"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParam is the test parameters for DesksCUJ.
type TestParam struct {
	BrowserType browser.Type
}

// Run runs the desks CUJ by opening up 4 different desks and switching
// between them using various workflows.
func Run(ctx context.Context, cr *chrome.Chrome, testParam TestParam, args func(string) (string, bool), outDir, systemTraceConfigPath string) (pv *perf.Values, retErr error) {
	deskCUJTestDuration := 10 * time.Minute
	if testDuration, ok := args("ui.DesksCUJ.duration"); ok {
		var err error
		deskCUJTestDuration, err = time.ParseDuration(testDuration)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse command-line arg ui.DesksCUJ.duration=%q", testDuration)
		}
	}

	// deskSwitchingDuration is how long we should run each workflow for.
	// To have the full test run in 10 minutes, we want to have each of
	// the 3 workflows run in 10/3 minutes.
	var deskSwitchingDuration = deskCUJTestDuration / 3

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return nil, errors.Wrap(err, "failed to capture device snapshot")
	}

	blankConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, testParam.BrowserType, chrome.BlankURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up Chrome")
	}
	defer closeBrowser(cleanupCtx)
	defer blankConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to browser test API connection")
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure clamshell mode")
	}
	defer cleanup(cleanupCtx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the keyboard")
	}
	defer kw.Close(ctx)

	mw, err := input.Mouse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the mouse")
	}
	defer mw.Close(ctx)

	tpw, err := input.Trackpad(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a trackpad device")
	}
	defer tpw.Close(ctx)

	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a multi-touch writer with 2 touches")
	}
	defer tw.Close()

	ac := uiauto.New(tconn)

	// The above preparation may take several minutes. Ensure that the
	// display is awake and will stay awake for the performance measurement.
	if err := power.TurnOnDisplay(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wake display")
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the recorder")
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to recorder")
	}

	// Take a screenshot every 2 minutes up to a maximum of 5
	// screenshots, to capture the state of the device during each of the
	// desk switching workflows.
	if err := recorder.AddScreenshotRecorder(ctx, 2*time.Minute, 5); err != nil {
		return nil, errors.Wrap(err, "failed to add screenshot recorder")
	}

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	// Shorten the context to cleanup document.
	// Some low-end devices take a long time to delete docs, so extend
	// timeout to one minute.
	cleanUpDeskCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Open all desks and windows for each desk. Additionally, initialize
	// unique user input actions that will be performed on each desk.
	onVisitActions, expectedNumWindows, cleanUpDesks, err := setUpDesks(ctx, tconn, bTconn, br, kw, mw, tpw, tw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up desks")
	}
	defer func(ctx context.Context) {
		if err := cleanUpDesks(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up desks: ", err)
		}
	}(cleanUpDeskCtx)

	topRow, err := input.KeyboardTopRowLayout(ctx, kw)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the top-row layout")
	}
	setOverviewModeAndWait := func(ctx context.Context) error {
		if err := kw.Accel(ctx, topRow.SelectTask); err != nil {
			return errors.Wrap(err, "failed to hit overview key")
		}
		return ash.WaitForOverviewState(ctx, tconn, ash.Shown, time.Minute)
	}
	deskSwitchWorkflows := []DeskSwitchWorkflow{
		GetKeyboardSearchBracketWorkflow(tconn, kw),
		GetKeyboardSearchNumberWorkflow(tconn, kw),
		GetOverviewWorkflow(tconn, ac, setOverviewModeAndWait),
	}

	if err := browser.CloseTabByTitle(ctx, bTconn, "about:blank"); err != nil {
		return nil, errors.Wrap(err, "failed to close blank tab")
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Open a window within recorder.Run to ensure we collect
		// PageLoad.PaintTiming.NavigationToFirstContentfulPaint.
		if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
			return errors.Wrap(err, "failed to activate leftmost desk with the autotest API")
		}
		activeDesk := 0

		recorder.Annotate(ctx, "Open_Google_Slides")
		slidesURL, err := cuj.GetTestSlidesURL(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get Google Slides URL")
		}

		slidesConn, err := recorder.NewConn(ctx, br, "Slides", slidesURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open a Google Slides presentation")
		}
		expectedNumWindows++

		deskSwitcher := NewDeskSwitcher(tconn, recorder, outDir, systemTraceConfigPath, deskSwitchingDuration, deskSwitchWorkflows, onVisitActions, expectedNumWindows, activeDesk)
		if err := deskSwitcher.DeskSwitch(ctx); err != nil {
			return errors.Wrap(err, "failed to perform desk switching action")
		}

		// Activate the desk where Google Slides is at.
		if deskSwitcher.ActiveDesk != 0 {
			if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
				return errors.Wrap(err, "failed to activate leftmost desk with the autotest API")
			}
		}
		const chromeVersionURL = chrome.VersionURL
		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := slidesConn.Navigate(ctx, chromeVersionURL); err != nil {
			if !strings.Contains(err.Error(), "the connection is closing") {
				return errors.Wrapf(err, "failed to navigate to %s", chromeVersionURL)
			}
			testing.ContextLog(ctx, "Attempt to reconnect to Google Slides after losing connection: ", err)
			// Sometimes the connection is lost due to tab discarding.
			// Try to reconnect it and navigate to chrome://version.
			matcher := func(t *target.Info) bool {
				return strings.Contains(t.URL, slidesURL)
			}
			slidesConn, err = br.NewConnForTarget(ctx, matcher)
			if err != nil {
				return errors.Wrap(err, "failed to reconnect to Google Slides tab")
			}
			if err := slidesConn.Navigate(ctx, chromeVersionURL); err != nil {
				return errors.Wrapf(err, "failed to navigate to %s", chromeVersionURL)
			}
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to conduct the recorder task")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return nil, errors.Wrap(err, "failed to record the performance data")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		testing.ContextLog(ctx, "Failed to save histogram raw data: ", err)
	}
	return pv, nil
}
