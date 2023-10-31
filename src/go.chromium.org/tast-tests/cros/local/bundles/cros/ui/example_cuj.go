// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Example of using the CUJ Recorder",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Timeout:      15 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: "loggedInToCUJUser",
				Val:     browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           "loggedInToCUJUserLacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

// ExampleCUJ runs an example CUJ test using the CUJ recorder. This
// test peforms specific actions that trigger metrics for first
// contentful paint (FCP), largest contentful paint (LCP), mouse
// latency, key press latency, and Ash smoothness.
//
// A video of this CUJ test can be found at go/examplecuj.
func ExampleCUJ(ctx context.Context, s *testing.State) {
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Access Chrome from the fixture, assuming that the fixture
	// implements the chrome.HasChrome interface.
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Use browserfixt.Setup to setup either Lacros or Ash, based on
	// the browser type set by the test variant parameters.
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), "about:blank")
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer conn.Close()

	// tconn is the Ash-chrome test connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	// bTconn is the browser test connection. This could either be a
	// connection to Ash or Lacros, depending on the test variant.
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Falied to connect to browser test API connection: ", err)
	}

	// Create a new recorder with cujrecorder.NewRecorder.
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	// [Optional] Add the pre-existing list of metrics to the
	// recorder with recorder.AddCommonMetrics.
	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to the recorder: ", err)
	}

	// Ensure we are in clamshell mode, because this example
	// primarily focuses on mouse actions, which are not valid
	// workflows on devices in tablet mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(closeCtx)

	// Since we are in clamshell mode, set up a mouse pointer to help
	// us click/drag with the mouse on the device. pointer.NewMouse is
	// associated with pointer.Context, which is a common interface for
	// both mouse and touch input. This interface is great for
	// dragging, but doesn't support moving the mouse, since there is
	// no corresponding touch input. To move the mouse, we need
	// input.Mouse.
	pc := pointer.NewMouse(tconn)
	defer pc.Close(ctx)

	// Create a virtual mouse. This mouse lets our test perform mouse
	// actions, such as moving, scrolling, pressing, and releasing.
	mw, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to get the mouse: ", err)
	}
	defer mw.Close(ctx)

	// Create a virtual keyboard. This keyboard lets our test send key
	// presses during the test.
	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(ctx)

	// Get the primary display info. In this particular test, this is
	// used to find the display bounds, for dragging the mouse to the
	// left and right of the screen.
	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	// The package uiauto contains multiple functions that help us
	// interact with the ui tree.
	ac := uiauto.New(tconn)

	// Use faillog.DumpUITreeWithScreenshotOnError to capture the
	// device state at the time of a failure. A screenshot and a text
	// file containing the ui tree are stored in the test out directory.
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// recorder.Run runs the provided function, and collects metrics
	// during its execution.
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		const (
			chromiumURL     = "https://chromium.org/Home"
			issueTrackerURL = "https://bugs.chromium.org/p/chromium/issues/list"
			searchQuery     = "This is my example search query for the Chromium website"
			testDuration    = 10 * time.Minute
		)

		// [Optional] Only record trace for one iteration.
		var traceDone bool

		// We want the test to run for about 10 minutes to collect
		// meaningful performance and power metrics. In this test, we
		// repeat the following actions until 10 minutes have passed.
		for endTime := time.Now().Add(testDuration); time.Now().Before(endTime); {
			// Navigate to the Chromium home page. Doing so
			// within recorder.Run generates FCP metrics.
			if err := conn.Navigate(ctx, chromiumURL); err != nil {
				return errors.Wrapf(err, "failed to navigate to %s", chromiumURL)
			}

			// Wait for the website to stabilize. If the page doesn't
			// stabilize in time, just keep going with the test.
			// Sometimes, due to transient issues, the website might not
			// fully quiesce, but the page is loaded enough to continue.
			if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
				s.Logf("Failed to wait for the tab %s to quiesce", chromiumURL)
			}

			// uiauto.Combine combines a series of actions into a
			// single action, which helps lessen the code verboseness
			// by combining error handling into a single line. The
			// following actions interact with the device by using the
			// ui tree to find the search box and the search modal, to
			// generate mouse move/press/release and key press metrics.
			search := nodewith.NameStartingWith("Search").Role("button")
			searchModal := nodewith.HasClass("DocSearch-Container")
			if err := uiauto.Combine(
				"open the search window, type a search query, and exit the search window",
				// Move mouse to search box and click it.
				ac.MouseMoveTo(search, 500*time.Millisecond),
				ac.LeftClick(search),

				// Wait for the search modal to appear.
				ac.WaitUntilExists(searchModal),

				// Since the search modal is popped up and focused,
				// type a query into that text box.
				kw.TypeAction(searchQuery),

				// Press the Esc key to exit the search modal.
				kw.AccelAction("Esc"),

				// Wait until the search modal is completely gone from
				// view before continuing. Without waiting until gone,
				// the next action might be unexpectedly performed on
				// the search modal.
				ac.WaitUntilGone(searchModal),
			)(ctx); err != nil {
				return err
			}

			// RunDragMouseCycle clicks at the center of the screen,
			// and drags the mouse to the left and right, and then back
			// to the center. This essentially highlights and
			// unhighlights text on the screen. This is a simple way to
			// generate mouse drag metrics.
			if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
				return errors.Wrap(err, "failed to run the drag mouse cycle")
			}

			// Navigate away from the the Chromium home page to the
			// Chromium bug tracker. Navigating away from the initial
			// page generates LCP metrics.
			if err := conn.Navigate(ctx, issueTrackerURL); err != nil {
				return errors.Wrapf(err, "failed to navigate to %s", issueTrackerURL)
			}

			if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
				s.Logf("Failed to wait for the tab %s to quiesce", issueTrackerURL)
			}

			// [Optional] Start tracing.
			// See go/trace-in-cuj-tests about rules for tracing.
			if !traceDone {
				if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
			}

			// Since the Chromium bug tracker is a scrollable page,
			// scroll up and down using the mouse wheel.
			for _, direction := range []string{"Down", "Up"} {
				// Scroll 50 mouse wheel ticks, with 50 milliseconds in
				// between ticks.
				if err := inputsimulations.RepeatMouseScroll(ctx, mw, direction == "Down", 50*time.Millisecond, 50); err != nil {
					return errors.Wrapf(err, "failed to repeat mouse scroll %s", direction)
				}

				// Press the down or up arrow key 15 times, with 200
				// milliseconds in between each press.
				if err := inputsimulations.RepeatKeyPress(ctx, kw, direction, 200*time.Millisecond, 15); err != nil {
					return errors.Wrapf(err, "failed to repeatedly press the %s arrow key", direction)
				}
			}

			// [Optional] Stop tracing.
			if !traceDone {
				if err := recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
				traceDone = true
			}

			// Generating Ash smoothness involves interacting with the
			// Ash UI. If the test doesn't organically interact with
			// the Ash UI, inputsimulations.DoAshWorkflows can provide
			// a canned workflow that can generate ADF metrics. In
			// particular, it toggles the system tray, and drags a
			// window in overview mode.
			if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
				return errors.Wrap(err, "failed to do Ash workflows")
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	// Recorder cleanup involves calling recorder.Record to get the
	// metrics in perf.Values, recorder.SaveTraceFiles to save trace
	// files, and pv.Save to save the metrics to the test out directory.
	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
