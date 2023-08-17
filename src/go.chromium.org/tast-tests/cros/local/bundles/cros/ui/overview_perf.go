// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OverviewPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures animation smoothness of entering/exiting the overview mode",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"oshima@chromium.org",
			"xiyuan@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "chromeLoggedIn",
			Timeout: 20 * time.Minute,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           20 * time.Minute,
		}, {
			Name:    "passthrough",
			Val:     browser.TypeAsh,
			Fixture: "chromeLoggedInWith100FakeAppsPassthroughCmdDecoder",
			Timeout: 20 * time.Minute,
		}},
		Data: []string{"animation.html", "animation.js"},
	})
}

func OverviewPerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	blankConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer blankConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}

	originalTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the tablet mode status: ", err)
	}
	defer ash.SetTabletModeEnabled(cleanupCtx, tconn, originalTabletMode)

	// A function, if assigned, will save the screenshot captured when
	// determining the split-screen capability. It is called when there are
	// test failures. The saved screenshot can help debugging.
	var saveCanSplitScreenshot func(context.Context) error
	// Take screenshot when s.Fatal() is called.
	s.AttachErrorHandlers(
		nil, // Skip the handler for s.Error().
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")
			if saveCanSplitScreenshot != nil {
				saveCanSplitScreenshot(cleanupCtx)
			}
		})
	canSplitScreen, err := func(ctx context.Context) (bool, error) {
		if originalTabletMode != true {
			// The tests also run on devices that do not support tablet mode. Thus,
			// in case of an error, just log it and return false.
			if err := ash.SetTabletModeEnabled(ctx, tconn, true); err != nil {
				testing.ContextLog(ctx, "Failed to set tablet mode: ", err)
				return false, nil
			}
			defer ash.SetTabletModeEnabled(ctx, tconn, originalTabletMode)
		}

		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return false, errors.Wrap(err, "failed to get windows")
		}
		// There should exist only one blank Chrome window now.
		if len(ws) != 1 {
			return false, errors.Errorf("got %d windows when one Chrome window is expected", len(ws))
		}

		// Wait until the Chrome buttons for tablet mode are ready by checking
		// the existence and stability of the "New tab" button.
		if err := uiauto.New(tconn).WaitForLocation(nodewith.ClassName("ToolbarButton").Name("New tab"))(ctx); err != nil {
			return false, errors.Wrap(err, "failed to wait for Chrome \"New tab\" button in tablet mode")
		}

		// Take the current screenshot. If the test fails, compare it with the
		// failure screenshot to find out Chrome UI changes.
		if img, err := screenshot.CaptureChromeImageWithTestAPI(ctx, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
		} else {
			saveCanSplitScreenshot = func(ctx context.Context) error {
				imgBuf := new(bytes.Buffer)
				if err := png.Encode(imgBuf, img); err != nil {
					return errors.Wrap(err, "failed to encode png image")
				}
				return os.WriteFile((filepath.Join(s.OutDir(), "ui_can_split.png")), imgBuf.Bytes(), 0644)
			}
		}

		win := ws[0]
		// Try to split the active window and see if it works on the current device.
		initialState := win.State
		if err := ash.SetWindowStateWithTimeout(ctx, tconn, win.ID, ash.WindowStatePrimarySnapped, 5*time.Second); err != nil {
			w, wErr := ash.GetWindow(ctx, tconn, win.ID)
			if wErr != nil {
				// Log the current error and return with the original error.
				testing.ContextLog(ctx, "Failed to get window: ", wErr)
				return false, errors.Wrap(err, "failed to snap window")
			}
			if w.State != initialState {
				return false, errors.Wrapf(err, "failed to snap window with state changed from %s to %s", initialState, w.State)
			}

			// On a device with small resolutions like 1200x675, it's normal
			// that Chrome cannot be snapped because the width of the split
			// screen is smaller than the minimum width of the Chrome window.
			// Just log the error and return false with nil.
			testing.ContextLogf(ctx, "Failed to snap window with state in %s: %v", w.State, err)
			return false, nil
		}
		// Restore the original window state.
		defer ash.SetWindowStateAndWait(ctx, tconn, win.ID, initialState)

		return true, nil
	}(ctx)
	if err != nil {
		s.Fatal("Failed to determine if Chrome can be split screened: ", err)
	}
	s.Log("Whether Chrome can be split screened in the current device: ", canSplitScreen)

	// Run an http server to serve the test contents for accessing from the chrome browsers.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	url := server.URL + "/animation.html"

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{Mode: cujrecorder.Perf})
	if err != nil {
		s.Fatal("Failed to create a CUJ recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		currentWindows := 0
		// Run the overview mode enter/exit flow for various situations.
		// - change the number of browser windows,
		// - the window system status; clamshell mode with maximized windows,
		//   clamshell mode with normal windows, tablet mode with maximized
		//   windows, tablet mode with minimized windows (the home screen),
		//   tablet split view with maximized overview windows, or tablet
		//   split view with minimized overview windows.
		for i, windows := range []int{2, 3, 4, 8} {
			// This assumes that the test scenarios are sorted by
			// number of windows. If not, then this will generate
			// Panic: runtime error: makeslice: cap out of range
			if err := ash.CreateWindows(ctx, tconn, br, url, windows-currentWindows); err != nil {
				return errors.Wrap(err, "failed to create browser windows")
			}

			// This must be done after ash.CreateWindows to avoid terminating lacros-chrome.
			if i == 0 {
				if err := br.CloseWithURL(ctx, chrome.BlankURL); err != nil {
					return errors.Wrap(err, "failed to close initial blank tab")
				}
			}

			currentWindows = windows

			windowsDescription := fmt.Sprintf("%dwindows", windows)
			for _, test := range []struct {
				fullDescriptionFmt  string
				tablet              bool
				overviewWindowState ash.WindowStateType
				histogramSuffix     string
			}{
				{"SingleClamshellMode-%dwindows", false, ash.WindowStateMaximized, "SingleClamshellMode"},
				{"ClamshellMode-%dwindows", false, ash.WindowStateNormal, "ClamshellMode"},
				{"TabletMode-%dwindows", true, ash.WindowStateMaximized, "TabletMode"},
				{"MinimizedTabletMode-%dwindows", true, ash.WindowStateMinimized, "MinimizedTabletMode"},
			} {
				fullDescription := fmt.Sprintf(test.fullDescriptionFmt, windows)
				if err := doTestCase(
					ctx, s, tconn, runner, fullDescription, windowsDescription, test.tablet, test.overviewWindowState,
					false /*splitview*/, "Ash.Overview.AnimationSmoothness.Enter."+test.histogramSuffix,
					"Ash.Overview.AnimationSmoothness.Exit."+test.histogramSuffix,
				); err != nil {
					return errors.Wrapf(err, "test case %q failed", fullDescription)
				}
			}

			// Some devices are unable to enter split screen, because the
			// display is too small. Skip split screening on those devices.
			if !canSplitScreen {
				continue
			}

			if windows == 2 {
				// The overview exit animation does not include the window being activated.
				// Thus, the SplitView-2windows case has no overview exit animation at all.
				if err := doTestCase(
					ctx, s, tconn, runner, "SplitView-2windows", "2windows", true /*tablet*/, ash.WindowStateMaximized,
					true /*splitview*/, "Ash.Overview.AnimationSmoothness.Enter.SplitView",
				); err != nil {
					s.Fatal("Test case \"SplitView-2windows\" failed: ", err)
				}
				continue
			}

			for _, test := range []struct {
				fullDescriptionFmt    string
				windowsDescriptionFmt string
				overviewWindowState   ash.WindowStateType
			}{
				{"SplitView-%dwindowsincludingmaximizedoverviewwindows", "%dwindowsincludingmaximizedoverviewwindows", ash.WindowStateMaximized},
				{"SplitView-%dwindowsincludingminimizedoverviewwindows", "%dwindowsincludingminimizedoverviewwindows", ash.WindowStateMinimized},
			} {
				fullDescription := fmt.Sprintf(test.fullDescriptionFmt, windows)
				windowsDescription := fmt.Sprintf(test.windowsDescriptionFmt, windows)
				if err := doTestCase(
					ctx, s, tconn, runner, fullDescription, windowsDescription, true /*tablet*/, test.overviewWindowState,
					true /*splitview*/, "Ash.Overview.AnimationSmoothness.Enter.SplitView",
					"Ash.Overview.AnimationSmoothness.Exit.SplitView",
				); err != nil {
					s.Fatalf("Test case %q failed: %s", fullDescription, err)
				}
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := runner.Values().Values(ctx)
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}

// doTestCase runs the overview entering/exiting test using the given arguments.
func doTestCase(
	ctx context.Context,
	s *testing.State,
	tconn *chrome.TestConn,
	runner *perfutil.Runner,
	fullDescription,
	windowsDescription string,
	tablet bool,
	overviewWindowState ash.WindowStateType,
	splitView bool,
	histograms ...string,
) error {
	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		return errors.Wrap(err, "failed to turn on display")
	}

	// Here we try to set tablet mode enabled/disabled,
	// but keep in mind that the tests run on devices
	// that do not support tablet mode. Thus, in case
	// of an error, we skip to the next scenario.
	if err := ash.SetTabletModeEnabled(ctx, tconn, tablet); err != nil {
		testing.ContextLogf(ctx, "Skipping the case of %s as it failed to set tablet mode %v: %v", fullDescription, tablet, err)
		return nil
	}

	// Set all windows to the state that
	// we want for the overview windows.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		return ash.SetWindowStateAndWait(ctx, tconn, w.ID, overviewWindowState)
	}); err != nil {
		return errors.Wrapf(err, "failed to set all windows to state %v", overviewWindowState)
	}

	// For tablet split view scenarios, snap
	// a window and then exit overview.
	if splitView {
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get windows")
		}

		if len(ws) == 0 {
			return errors.Wrap(err, "found no windows")
		}

		if err := ash.SetWindowStateAndWait(ctx, tconn, ws[0].ID, ash.WindowStatePrimarySnapped); err != nil {
			return errors.Wrap(err, "failed to snap window")
		}

		if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
			return errors.Wrap(err, "failed to exit overview")
		}
	}

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(3*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Log("Failed to wait for overview stabilization: ", err)
	}

	runner.RunMultiple(ctx, fullDescription, uiperf.Run(s,
		perfutil.RunAndWaitAll(tconn,
			func(ctx context.Context) error {
				if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
					return errors.Wrap(err, "failed to enter into the overview mode")
				}
				if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
					return errors.Wrap(err, "failed to exit from the overview mode")
				}
				return nil
			},
			histograms...,
		)),
		perfutil.StoreAll(perf.BiggerIsBetter, "percent", windowsDescription))

	return nil
}
