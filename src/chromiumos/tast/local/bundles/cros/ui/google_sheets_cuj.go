// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/cuj/inputsimulations"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/mouse"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/pointer"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleSheetsCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the total performance of critical user journey for Google Sheets",
		Contacts: []string{
			"chromeos-perfmetrics-eng@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "loggedInToCUJUser",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "loggedInToCUJUserLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

func GoogleSheetsCUJ(ctx context.Context, s *testing.State) {
	const (
		timeout                 = 10 * time.Second
		overallScrollTimeout    = 10 * time.Minute
		individualScrollTimeout = overallScrollTimeout / 4
	)

	sheetURL, err := cuj.GetTestSheetsURL(ctx)
	if err != nil {
		s.Fatal("Failed to get Google Sheets URL: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	sheetConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer sheetConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to detect it is in tablet-mode or not: ", err)
	}

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close()
	s.Logf("Is in tablet-mode: %t", inTabletMode)

	ui := uiauto.New(tconn)

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a CUJ recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	// Create a virtual trackpad.
	tpw, err := input.Trackpad(ctx)
	if err != nil {
		s.Fatal("Failed to create a trackpad device: ", err)
	}
	defer tpw.Close()
	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		s.Fatal("Failed to create a multi touch writer: ", err)
	}
	defer tw.Close()

	// Create a virtual keyboard.
	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close()

	// Create a virtual mouse.
	mw, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create a mouse: ", err)
	}
	defer mw.Close()

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	defer faillog.DumpUITreeOnError(closeCtx, s.OutDir(), s.HasError, tconn)

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Start tracing now.
		if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
			return errors.Wrap(err, "failed to start tracing")
		}
		defer recorder.StopTracing(ctx)

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

		s.Logf("Scrolling down the Google Sheets file for %s", overallScrollTimeout)

		for _, scroller := range []struct {
			// description is a string that can be used with
			// cujrecorder.Recorder.Annotate to describe the scroll method.
			description string

			// run is a function that performs a single instance of scrolling.
			run action.Action
		}{
			{
				description: "mouse_click",
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
			},
			{
				description: "mouse_wheel",
				run: func(ctx context.Context) error {
					return inputsimulations.ScrollMouseDownFor(ctx, mw, 200*time.Millisecond, individualScrollTimeout)
				},
			},
			{
				description: "trackpad_gestures",
				run: func(ctx context.Context) error {
					return inputsimulations.ScrollDownFor(ctx, tpw, tw, 500*time.Millisecond, individualScrollTimeout)
				},
			},
			{
				description: "key_press",
				run: func(ctx context.Context) error {
					return inputsimulations.RepeatKeyPressFor(ctx, kw, "Down", 500*time.Millisecond, individualScrollTimeout)
				},
			},
		} {
			recorder.Annotate(ctx, "Scroll_with_"+scroller.description)
			if err := scroller.run(ctx); err != nil {
				return errors.Wrapf(err, "failed to scroll %s", scroller.description)
			}

			if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
				return err
			}

			if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
				return errors.Wrap(err, "failed to do Ash workflows")
			}
		}

		// Take a screenshot to see the state of the Google
		// Sheet after scrolling for 10 minutes.
		recorder.CustomScreenshot(ctx)

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
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data: ", err)
	}
}
