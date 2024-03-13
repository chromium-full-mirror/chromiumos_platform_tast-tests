// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package googlesheetscuj contains the test code for GoogleSheetsCUJ.
package googlesheetscuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParam is the test parameters for GoogleSheetsCUJ.
type TestParam struct {
	BrowserType browser.Type
}

// Run opens up a Google Sheets file, and use mousewheel/trackpad/keypress to
// scroll the sheets file, to test the Google Sheets performance.
func Run(ctx context.Context, cr *chrome.Chrome, testParam TestParam, outDir, systemTraceConfigPath string) (pv *perf.Values, retErr error) {
	const (
		timeout                 = 10 * time.Second
		overallScrollTimeout    = 10 * time.Minute
		individualScrollTimeout = overallScrollTimeout / 4
		imageCopyRepeatTimes    = 150
	)

	sheetURL, err := cuj.GetTestSheetsURL(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Google Sheets URL")
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pv, err = localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return nil, errors.Wrap(err, "failed to capture device snapshot")
	}

	sheetConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, testParam.BrowserType, chrome.BlankURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup Chrome")
	}
	defer closeBrowser(closeCtx)
	defer sheetConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to browser test API connection")
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect it is in tablet-mode or not")
	}

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create a touch controller")
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)
	testing.ContextLogf(ctx, "Is in tablet-mode: %t", inTabletMode)

	ui := uiauto.New(tconn)

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a CUJ recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to recorder")
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}

	// Get a small set of metrics to track across each scroll phase.
	ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()

	// Create a virtual trackpad.
	tpw, err := input.Trackpad(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a trackpad device")
	}
	defer tpw.Close(ctx)
	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a multi touch writer")
	}
	defer tw.Close()

	// Create a virtual keyboard.
	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a keyboard")
	}
	defer kw.Close(ctx)

	// Create a virtual mouse.
	mw, err := input.Mouse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a mouse")
	}
	defer mw.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the primary display info")
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	if err := cuj.WaitForValidAccountInCookieJar(ctx, br, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to wait for valid account in cookie jar")
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Open Google Sheets file.
		recorder.Annotate(ctx, "Opening_Google_Sheets_file")
		if err := sheetConn.Navigate(ctx, sheetURL); err != nil {
			return errors.Wrapf(err, "failed to navigate to %s", sheetURL)
		}

		// Pop-up content regarding view history privacy might show up.
		privacyButton := nodewith.Name("I understand").Role(role.Button)
		if err := uiauto.IfSuccessThen(ui.WaitUntilExists(privacyButton), ui.LeftClick(privacyButton))(ctx); err != nil {
			return errors.Wrap(err, "failed to click the spreadsheet privacy button")
		}

		testing.ContextLogf(ctx, "Scrolling down the Google Sheets file for %s", overallScrollTimeout)

		for _, scroller := range []struct {
			// description is a string that can be used with
			// cujrecorder.Recorder.Annotate to describe the scroll method.
			description string

			// snapshotPrefix is a string prefix for the snapshot metrics.
			snapshotPrefix string

			// run is a function that performs a single instance of scrolling.
			run action.Action

			// recordTrace indicates whether to record trace.
			recordTrace bool
		}{
			{
				description:    "mouse_click",
				snapshotPrefix: "ScrollMouseClick",
				run: func(ctx context.Context) error {
					sheetBounds, err := ui.Location(ctx, nodewith.Role("genericContainer").HasClass("grid-scrollable-wrapper"))
					if err != nil {
						return errors.Wrap(err, "failed to get the sheet location on the display")
					}

					// Select the point slightly to the right and above the bottom
					// corner of the sheet bounds. This is where the down arrow is.
					scrollArrowOffset := coords.NewPoint(4, -4)
					downArrow := sheetBounds.BottomRight().Add(scrollArrowOffset)
					if err := mouse.Move(tconn, downArrow, time.Second)(ctx); err != nil {
						return errors.Wrap(err, "failed to move mouse to the down arrow")
					}

					return inputsimulations.RepeatMousePressFor(ctx, mw, 500*time.Millisecond, 3*time.Second, individualScrollTimeout)
				},
				recordTrace: true,
			},
			{
				description:    "mouse_wheel",
				snapshotPrefix: "ScrollMouseWheel",
				run: func(ctx context.Context) error {
					return inputsimulations.ScrollMouseDownFor(ctx, mw, 200*time.Millisecond, individualScrollTimeout)
				},
			},
			{
				description:    "trackpad_gestures",
				snapshotPrefix: "ScrollTrackpadGestures",
				run: func(ctx context.Context) error {
					return inputsimulations.ScrollDownFor(ctx, tpw, tw, 500*time.Millisecond, individualScrollTimeout)
				},
			},
			{
				description:    "key_press",
				snapshotPrefix: "ScrollKeyPress",
				run: func(ctx context.Context) error {
					return inputsimulations.RepeatKeyPressFor(ctx, kw, "Down", 500*time.Millisecond, individualScrollTimeout)
				},
			},
		} {
			// Close any potential security alert that pops up.
			if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, sheetConn); err != nil {
				return errors.Wrap(err, "failed to dismiss Critical Security Alert")
			}

			recorder.Annotate(ctx, "Scroll_with_"+scroller.description)

			// See go/trace-in-cuj-tests about rules for tracing.
			if scroller.recordTrace {
				if err := recorder.StartTracing(ctx, outDir, systemTraceConfigPath); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
			}

			stopSnapshot, err := recorder.StartSnapshot(ctx, scroller.snapshotPrefix, ashMetrics, browserMetrics)
			if err != nil {
				return errors.Wrapf(err, "failed to start snapshot for %s", scroller.description)
			}

			if err := scroller.run(ctx); err != nil {
				return errors.Wrapf(err, "failed to scroll %s", scroller.description)
			}

			if err := stopSnapshot(ctx); err != nil {
				return errors.Wrapf(err, "failed to stop snapshot for %s", scroller.description)
			}

			if scroller.recordTrace {
				if err := recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
			}

			if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
				return errors.Wrap(err, "failed to do Ash workflows")
			}

			if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
				return err
			}

			// Take a screenshot to see the state of the Google
			// Sheet after scrolling.
			recorder.CustomScreenshot(ctx)
		}

		var scrollTop int
		// Ensure scrollbar gets scrolled.
		if err := sheetConn.Eval(ctx, "parseInt(document.getElementsByClassName('native-scrollbar-y')[0].scrollTop)", &scrollTop); err != nil {
			return errors.Wrap(err, "failed to get the number of pixels that the scrollbar is scrolled vertically")
		}
		if scrollTop == 0 {
			return errors.New("scroll didn't happen")
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := sheetConn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		// Ensure that there is exactly 1 window open at the end of the test.
		if ws, err := ash.GetAllWindows(ctx, tconn); len(ws) != 1 {
			return errors.Wrapf(err, "unexpected number of open windows, got: %d, expected: 1", len(ws))
		}

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to run the test scenario")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return nil, errors.Wrap(err, "failed to record the data")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	// Try to save both perf data and histogram raw data and return the first error if any.
	var saveDataErr error
	if err := pv.Save(outDir); err != nil {
		testing.ContextLog(ctx, "Failed to save the perf data: ", err)
		saveDataErr = errors.Wrap(err, "failed to save the perf data")
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		testing.ContextLog(ctx, "Failed to save histogram raw data: ", err)
		if saveDataErr == nil {
			saveDataErr = errors.Wrap(err, "failed to save histogram raw data")
		}
	}
	return pv, saveDataErr
}
