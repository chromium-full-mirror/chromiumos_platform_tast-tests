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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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

	if err := browser.CloseTabByTitle(ctx, bTconn, "about:blank"); err != nil {
		return nil, errors.Wrap(err, "failed to close blank tab")
	}

	// Get a list of metrics to collect for each test phase.
	ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()
	ashMetrics = append(ashMetrics, "Ash.Desks.AnimationLatency.DeskActivation", "Ash.Desks.AnimationSmoothness.DeskActivation")

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

		for _, deskSwitcher := range []deskSwitchWorkflow{
			getKeyboardSearchBracketWorkflow(tconn, kw),
			getKeyboardSearchNumberWorkflow(tconn, kw),
			getOverviewWorkflow(tconn, ac, setOverviewModeAndWait),
		} {
			cycles := 0

			if startDesk := deskSwitcher.itinerary[0]; activeDesk != startDesk {
				if err := ash.ActivateDeskAtIndex(ctx, tconn, startDesk); err != nil {
					return errors.Wrapf(err, "failed to activate desk %d with the autotest API", startDesk)
				}
				activeDesk = startDesk
			}

			recorder.Annotate(ctx, "Cycle_through_desks_with_"+deskSwitcher.name)

			stopSnapshot, err := recorder.StartSnapshot(ctx, deskSwitcher.name, ashMetrics, browserMetrics)
			if err != nil {
				return errors.Wrapf(err, "failed to start snapshot for %s", deskSwitcher.name)
			}

			i := 0
			for endTime := time.Now().Add(deskSwitchingDuration); time.Now().Before(endTime); {
				// Record trace from starting cycling with search-bracket to completing 4 cycles.
				// See go/trace-in-cuj-tests about rules for tracing.
				if deskSwitcher.recordTrace {
					if cycles == 0 {
						if err := recorder.StartTracing(ctx, outDir, systemTraceConfigPath); err != nil {
							return errors.Wrap(err, "failed to start tracing")
						}
					} else if cycles == 4 {
						if err := recorder.StopTracing(ctx); err != nil {
							return errors.Wrap(err, "failed to stop tracing")
						}
					}
				}
				i = (i + 1) % len(deskSwitcher.itinerary)
				nextDesk := deskSwitcher.itinerary[i]

				err := deskSwitcher.run(ctx, activeDesk, nextDesk)
				if err != nil {
					return errors.Wrapf(err, "failed to switch to the next desk using %s", deskSwitcher.name)
				}

				if err := ash.WaitForDesk(tconn, nextDesk)(ctx); err != nil {
					return errors.Wrapf(err, "failed to wait for the %d desk to be active", nextDesk)
				}
				activeDesk = nextDesk

				// Give a few seconds for the current desk to stabilize
				// before interacting with it.
				if err := ac.WithInterval(time.Second).WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to wait for current desk to stabilize: ", err)
				}

				if err := onVisitActions[activeDesk](ctx); err != nil {
					return errors.Wrapf(err, "failed to perform unique action on desk %d", activeDesk)
				}
				cycles++
			}

			if err := stopSnapshot(ctx); err != nil {
				return errors.Wrapf(err, "failed to stop snapshot for %s", deskSwitcher.name)
			}

			// Ensure that none of the windows crashed during the test.
			ws, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to get all windows")
			}

			if len(ws) != expectedNumWindows {
				return errors.Errorf("unexpected number of open windows, got %d, expected %d", len(ws), expectedNumWindows)
			}

			testing.ContextLogf(ctx, "Switched desk by %s %d times", deskSwitcher.name, cycles)
		}

		// Activate the desk where Google Slides is at.
		if activeDesk != 0 {
			if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
				return errors.Wrap(err, "failed to activate leftmost desk with the autotest API")
			}
			activeDesk = 0
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
