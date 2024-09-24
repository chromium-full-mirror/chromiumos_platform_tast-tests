// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package deskscujv2 contains helper util and test code for DesksCUJV2.
package deskscujv2

import (
	"context"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/deskscuj"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParam is the test parameters for DesksCUJV2.
type TestParam struct {
	AnimationURL string
	TestDuration time.Duration
}

// Run runs the desks CUJ V2 by opening up 4 different desks and switching
// between them using various workflows.
func Run(ctx context.Context, cr *chrome.Chrome, testParam TestParam, outDir, systemTraceConfigPath string) (pv *perf.Values, retErr error) {
	// DeskSwitchingDuration is how long we should run each workflow for.
	// To have the full test run in 10 minutes, we want to have each of
	// the 2 workflows run in 10/2 minutes.
	deskSwitchingDuration := testParam.TestDuration / 2

	// Reserve 15 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return nil, errors.Wrap(err, "failed to capture device snapshot")
	}

	blankConn, err := cr.NewConn(ctx, chrome.BlankURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up Chrome")

	}
	defer blankConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
	}

	br := cr.Browser()

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ensure clamshell mode")
	}
	defer cleanup(cleanupCtx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the keyboard")
	}
	defer kw.Close(cleanupCtx)

	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	mw, err := input.Mouse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the mouse")
	}
	defer mw.Close(cleanupCtx)

	tpw, err := input.Trackpad(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a trackpad device")
	}
	defer tpw.Close(cleanupCtx)

	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a multi-touch writer with 2 touches")
	}
	defer tw.Close()

	// The above preparation may take several minutes. Ensure that the
	// display is awake and will stay awake for the performance measurement.
	if err := power.TurnOnDisplay(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to turn on display")
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the recorder")
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to recorder")
	}

	// Take a screenshot every 2 minutes up to a maximum of 5
	// screenshots, to capture the state of the device during each of the
	// desk switching workflows.
	if err := recorder.AddScreenshotRecorder(ctx, 2*time.Minute, 5); err != nil {
		return nil, errors.Wrap(err, "failed to add screenshot recorder")
	}

	defer ash.CloseAllWindows(cleanupCtx, tconn)
	defer ash.CleanUpDesks(cleanupCtx, tconn)

	// Shorten the context to cleanup document.
	// Some low-end devices take a long time to delete docs, so extend
	// timeout to one minute.
	cleanUpDeskCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Open all desks and windows for each desk. Additionally, initialize
	// unique user input actions that will be performed on each desk.
	onVisitActions, expectedNumWindows, cleanUpDesks, err := setUpDesks(ctx, cr, tconn, br, kw, pc, mw, tpw, tw, testParam)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up desks")
	}
	defer func(ctx context.Context) {
		if err := cleanUpDesks(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up desks: ", err)
		}
	}(cleanUpDeskCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanUpDeskCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	ui := uiauto.New(tconn)

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
	deskSwitchWorkflows := []deskscuj.DeskSwitchWorkflow{
		deskscuj.GetKeyboardSearchBracketWorkflow(tconn, kw),
		deskscuj.GetOverviewWorkflow(tconn, ui, setOverviewModeAndWait),
	}

	if err := browser.CloseTabByTitle(ctx, tconn, "about:blank"); err != nil {
		return nil, errors.Wrap(err, "failed to close blank tab")
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Open a tab within recorder.Run to ensure we collect
		// PageLoad.PaintTiming.NavigationToFirstContentfulPaint.
		if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
			return errors.Wrap(err, "failed to activate leftmost desk with the autotest API")
		}
		activeDesk := 0

		recorder.Annotate(ctx, "Open_CrosVideo")
		videoConn, err := recorder.NewConn(ctx, br, "CrosVideo", crosVideoURL)
		if err != nil {
			return errors.Wrap(err, "failed to open CrosVideo website")
		}

		deskSwitcher := deskscuj.NewDeskSwitcher(tconn, recorder, outDir, systemTraceConfigPath, deskSwitchingDuration, deskSwitchWorkflows, onVisitActions, expectedNumWindows, activeDesk)
		if err := deskSwitcher.DeskSwitch(ctx); err != nil {
			return errors.Wrap(err, "failed to perform desk switching action")
		}

		// Activate the desk with CrosVideo.
		if deskSwitcher.ActiveDesk != 0 {
			if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
				return errors.Wrap(err, "failed to activate leftmost desk with the autotest API")
			}
		}
		const chromeVersionURL = chrome.VersionURL
		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := videoConn.Navigate(ctx, chromeVersionURL); err != nil {
			if !strings.Contains(err.Error(), "the connection is closing") {
				return errors.Wrapf(err, "failed to navigate to %s", chromeVersionURL)
			}
			testing.ContextLog(ctx, "Attempt to reconnect to CrosVideo after losing connection: ", err)
			// Sometimes the connection is lost due to tab discarding.
			// Try to reconnect it and navigate to chrome://version.
			matcher := func(t *target.Info) bool {
				return strings.Contains(t.URL, crosVideoURL)
			}
			videoConn, err = br.NewConnForTarget(ctx, matcher)
			if err != nil {
				return errors.Wrap(err, "failed to reconnect to CrosVideo tab")
			}
			if err := videoConn.Navigate(ctx, chromeVersionURL); err != nil {
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
