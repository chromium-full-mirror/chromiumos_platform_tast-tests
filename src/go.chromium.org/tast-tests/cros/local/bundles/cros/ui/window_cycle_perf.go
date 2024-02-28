// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/ui"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowCyclePerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the animation smoothness of window cycle animations when Alt + tabbing",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"chromeos-wm-corexp@google.com",
			"sammiequon@chromium.org",
			"xiyuan@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      cujrecorder.CooldownTimeout + 3*time.Minute,
		Params: []testing.Param{{
			Val:               browser.TypeAsh,
			Fixture:           "chromeLoggedIn",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			Fixture:           "lacros",
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			// Pilot test on "noibat" that has HDMI dongle installed.
			Name:              "noibat",
			Val:               browser.TypeAsh,
			Fixture:           "chromeLoggedIn",
			ExtraHardwareDeps: hwdep.D(hwdep.Model("noibat")),
		}},
	})
}

func WindowCyclePerf(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	initialSnapshot, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

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

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(ctx)

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	numExistingWindows := 0

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		Mode:              cujrecorder.Benchmark,
		CooldownBeforeRun: true,
	})
	if err != nil {
		s.Fatal("Failed to create a CUJ recorder: ", err)
	}
	defer recorder.Close(cleanupCtx)

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		for i, numWindows := range []int{2, 8, 16} {
			if err := ash.CreateWindows(ctx, tconn, br, ui.PerftestURL, numWindows-numExistingWindows); err != nil {
				s.Fatal("Failed to open browser windows: ", err)
			}

			// This must be done after ash.CreateWindows to avoid terminating lacros-chrome.
			if i == 0 {
				if err := br.CloseWithURL(ctx, chrome.BlankURL); err != nil {
					return errors.Wrap(err, "failed to close initial blank tab")
				}
			}

			// Maximize all windows to ensure a consistent state.
			if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
				return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
			}); err != nil {
				s.Fatal("Failed to maximize windows: ", err)
			}

			numExistingWindows = numWindows

			suffix := fmt.Sprintf("%dwindows", numWindows)
			// storeAllMetricsAndLogError stores all metrics to |pv| and print logs if there is any error.
			storeAllMetricsAndLogError := func(ctx context.Context, pv *perfutil.Values, hists []*histogram.Histogram) error {
				for _, hist := range hists {
					if err := perfutil.StoreMetricWithHeuristics(ctx, pv, hist, suffix); err != nil {
						// Log the error and keep storing other metrics.
						testing.ContextLogf(ctx, "Failed to store metric %q: %v", hist.Name, err)
					}
				}
				return nil
			}

			runner.RunMultiple(ctx, suffix, uiperf.Run(s, perfutil.RunAndWaitAny(tconn, func(ctx context.Context) error {
				// Create a shorter context to ensure the time to release the Alt-key.
				sctx, cancel := ctxutil.Shorten(ctx, 500*time.Millisecond)
				defer cancel()

				// First long press Alt + Tab to bring up the window cycle view.
				if err := keyboard.AccelPress(sctx, "Alt"); err != nil {
					return errors.Wrap(err, "failed to press alt")
				}
				defer keyboard.AccelRelease(ctx, "Alt")

				// GoBigSleepLint: Sleep to wait for the window lists to appear.
				if err := testing.Sleep(sctx, 500*time.Millisecond); err != nil {
					return errors.Wrap(err, "failed to wait")
				}
				if err := keyboard.Accel(sctx, "Tab"); err != nil {
					return errors.Wrap(err, "failed to type tab")
				}

				for i := 0; i < numWindows*2; i++ {
					if err := keyboard.Accel(sctx, "Tab"); err != nil {
						return errors.Wrap(err, "failed to type tab")
					}

					// GoBigSleepLint: Sleep to wait for the Alt+Tab animation
					// to complete. Use sleeps to ensure timing consistency
					// between test runs.
					if err := testing.Sleep(sctx, 200*time.Millisecond); err != nil {
						return errors.Wrap(err, "failed to wait")
					}
				}

				// GoBigSleepLint: Sleep in between each test variation.
				if err := testing.Sleep(sctx, time.Second); err != nil {
					return errors.Wrap(err, "failed to wait")
				}
				return nil
			},
				"Ash.WindowCycleView.AnimationSmoothness.Show",
				"Ash.WindowCycleView.AnimationSmoothness.Container",
				"Ash.WindowCycleController.Enter.PresentationTime")),
				storeAllMetricsAndLogError)
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := runner.Values().Values(ctx)
	pv.Merge(initialSnapshot)
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}
