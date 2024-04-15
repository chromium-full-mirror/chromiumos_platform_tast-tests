// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
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
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleSlidesCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the total performance of critical user journey for Google Slides",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Data:         []string{cujrecorder.SystemTraceConfigFile},
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

func GoogleSlidesCUJ(ctx context.Context, s *testing.State) {
	const slidesScrollTimeout = 10 * time.Minute

	slidesURL, err := cuj.GetTestSlidesURL(ctx)
	if err != nil {
		s.Fatal("Failed to get Google Slides URL: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	slidesConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer slidesConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

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

	// Create a virtual keyboard.
	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to detect if the device is in tablet mode or not: ", err)
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
	defer pc.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := cuj.WaitForValidAccountInCookieJar(ctx, br, tconn); err != nil {
		s.Fatal("Failed to wait for valid account in cookie jar: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Open Google Slides file.
		if err := slidesConn.Navigate(ctx, slidesURL); err != nil {
			return errors.Wrapf(err, "failed to navigate to %s", slidesURL)
		}

		// Go through the Slides deck.
		s.Logf("Going through the Google Slides file for %s", slidesScrollTimeout)

		ac := uiauto.New(tconn)
		slidesRootWebArea := nodewith.NameContaining("Google Slides").Role(role.RootWebArea)
		documentContentFocused := nodewith.Name("Document content").Role(role.TextField).Ancestor(slidesRootWebArea).Focused()
		if err := uiauto.Combine("click web area to focus on web content",
			ac.LeftClick(slidesRootWebArea),
			// Click at |slidesRootWebArea| might focus on the web area or
			// the document content.
			// Both focused status indicate the web content is focused.
			ac.WaitUntilAnyExists(slidesRootWebArea.Focused(), documentContentFocused),
		)(ctx); err != nil {
			return err
		}

		// Keep track of the number of iterations we run the
		// scroll-down cycle for. This allows us to control how often
		// we want to run Ash UI interactions to ensure collection of
		// Ash.Smoothness.PercentDroppedFrames_1sWindow.
		i := 0
		for endTime := time.Now().Add(slidesScrollTimeout); time.Now().Before(endTime); {
			// See go/trace-in-cuj-tests about rules for tracing.
			if i == 0 {
				if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
			}

			if err := inputsimulations.RepeatKeyPress(ctx, kw, "Down", time.Second, 10); err != nil {
				return errors.Wrap(err, "failed to scroll down with down arrow")
			}

			// Close any potential security alert that pops up.
			if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, slidesConn); err != nil {
				return errors.Wrap(err, "failed to dismiss Critical Security Alert")
			}

			// At fixed intervals, stop scrolling and click a menu item
			// to ensure we collect mouse metrics.
			understandBtn := nodewith.Name("I understand").Role(role.Button)
			fileMenu := nodewith.Name("File").HasClass("menu-button")
			if err := action.Combine(
				"open and close the file menu and then refocus on the presentation",
				uiauto.IfSuccessThen(
					ac.WithTimeout(5*time.Second).WaitUntilExists(understandBtn),
					ac.LeftClick(understandBtn),
				),
				// Open file menu.
				ac.MouseMoveTo(fileMenu, 500*time.Millisecond),
				ac.LeftClick(fileMenu),
				action.Sleep(time.Second),

				// Close file menu.
				ac.LeftClick(fileMenu),

				// Refocus on the presentation.
				mouse.Move(tconn, info.Bounds.CenterPoint(), 500*time.Millisecond),
			)(ctx); err != nil {
				return err
			}

			if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
				return err
			}

			// Interact with the Ash UI every 5 scroll iterations, beginning
			// with the first scroll iteration.
			if i%5 == 0 {
				if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
					return errors.Wrap(err, "failed to do Ash workflows")
				}
			}

			if i == 0 {
				if err := recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
			}

			i++
		}

		// Take a screenshot to see the state of the slide deck
		// after scrolling for 10 minutes.
		recorder.CustomScreenshot(ctx)

		// Ensure the slides deck gets scrolled.
		var scrollTop int
		if err := slidesConn.Eval(ctx, "parseInt(document.getElementsByClassName('punch-filmstrip-scroll')[0].scrollTop)", &scrollTop); err != nil {
			return errors.Wrap(err, "failed to get the number of pixels that the scrollbar is scrolled vertically")
		}
		if scrollTop == 0 {
			return errors.New("file is not getting scrolled")
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := slidesConn.Navigate(ctx, "chrome://version"); err != nil {
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
