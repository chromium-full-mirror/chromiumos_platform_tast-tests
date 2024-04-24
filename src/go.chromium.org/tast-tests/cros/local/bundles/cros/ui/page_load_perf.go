// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PageLoadPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures FCP and LCP performance",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"ramsaroop@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10*time.Minute + cujrecorder.CooldownTimeout,
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild", "crosbolt_fsi_check"},
				Val:       browser.TypeAsh,
				Fixture:   "tabSwitchCUJWPRAsh",
			}, {
				Name:              "lacros",
				Val:               browser.TypeLacros,
				Fixture:           "tabSwitchCUJWPRLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func PageLoadPerf(ctx context.Context, s *testing.State) {
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(browser.Type), chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to set up Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer conn.Close()

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

	runner := perfutil.NewRunner(cr.Browser(), perfutil.RunnerOptions{
		IgnoreFirstRun:   true,
		DropMinMaxValues: true,
	})

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		// FCP and LCP are already noisy metrics, so use the recorder mode
		// with the least amount of overhead.
		Mode:              cujrecorder.Benchmark,
		CooldownBeforeRun: true,
	})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	perfettoCfgPath := s.DataPath(cujrecorder.SystemTraceConfigFile)

	// Pick URLs that have already been cached by the TabSwitchCUJ WPR.
	pages := []struct {
		url    string
		prefix string
	}{
		{
			url:    "https://www.cnn.com/2020/04/15/tech/iphone-se/index.html",
			prefix: "CNN",
		},
		// TODO(b/296939495) Add a media centric page and a lighter page.
	}

	var initialSnapshot *perf.Values
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		initialSnapshot, err = localPerf.CaptureDeviceSnapshot(ctx, "Initial")
		if err != nil {
			return errors.Wrap(err, "failed to capture device snapshot")
		}

		for _, page := range pages {
			runner.RunMultiple(ctx, page.prefix, uiperf.Run(s,
				perfutil.RunAndWaitAll(bTconn,
					navigateToPage(ctx, conn, page.url, recorder),
					[]string{
						"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
						"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
					}...,
				)),
				perfutil.StoreAll(perf.SmallerIsBetter, "ms", ""),
			)

			// Use the recorder to navigate to the page an additional time to
			// capture a perfetto trace. Capture loading and blink trace
			// data alongside the recorder default categories.
			if err := recorder.StartTracingWithExtraCategories(ctx, s.OutDir(), page.prefix+".data.gz", perfettoCfgPath, "loading", "blink"); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}

			if err := navigateToPage(ctx, conn, page.url, recorder)(ctx); err != nil {
				return err
			}

			if err := recorder.StopTracing(ctx); err != nil {
				return errors.Wrap(err, "failed to stop tracing")
			}
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

	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
}

// navigateToPage navigates to |url|, wait for |conn| to quiese, and then
// navigate away. The first navigation gives FCP, and the second one gives LCP
// metrics.
func navigateToPage(ctx context.Context, conn *chrome.Conn, url string, recorder *cujrecorder.Recorder) func(ctx context.Context) error {
	return func(ctx context.Context) (e error) {
		defer func(ctx context.Context) error {
			if e != nil {
				recorder.CustomScreenshot(ctx)
			}
			return nil
		}(ctx)
		if err := conn.Navigate(ctx, url); err != nil {
			testing.ContextLogf(ctx, "Failed to navigate to %s: %v", url, err)
			return err
		}

		if err := webutil.WaitForQuiescence(ctx, conn, 20*time.Second); err != nil {
			testing.ContextLogf(ctx, "Failed to wait for the tab %s to quiesce", url)
		}

		if err := conn.Navigate(ctx, chrome.BlankURL); err != nil {
			testing.ContextLog(ctx, "Failed to navigate to about:blank: ", err)
			return err
		}
		return nil
	}
}
