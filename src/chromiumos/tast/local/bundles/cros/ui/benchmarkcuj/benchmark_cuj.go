// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package benchmarkcuj contains helper util and test code for BenchmarkCUJ.
package benchmarkcuj

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

const benchmarkPrefix = "Benchmark."

// BenchmarkTest holds parameters for the BenchmarkCUJ test variants.
type BenchmarkTest struct {
	BrowserType   browser.Type
	BenchmarkInfo benchmarkInfo
}

// benchmarkInfo will contain benchmark specific information.
type benchmarkInfo struct {
	name           string
	windowState    ash.WindowStateType
	benchmarkURL   string
	benchmarkSetUp func(context.Context, *browser.TestConn) error
	benchmarkRun   func(context.Context, *chrome.Conn) error
	benchmarkScore func(context.Context, *chrome.Conn, map[string]float64) error
}

// Run runs the Benchmark CUJ by running the benchmark and recording the result.
func Run(ctx context.Context, s *testing.State) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	testParam := s.Param().(BenchmarkTest)
	benchmarkParam := testParam.BenchmarkInfo

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	benchmarkConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr,
		testParam.BrowserType, benchmarkParam.benchmarkURL)
	if err != nil {
		s.Fatalf("Failed to setup Chrome with %s: %v", benchmarkParam.benchmarkURL, err)
	}
	defer closeBrowser(closeCtx)
	defer benchmarkConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Falied to connect to browser test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Set window to benchmark's preferred window state.
	windows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get all windows: ", err)
	}

	if len(windows) != 1 {
		s.Fatalf("Unexpected number of windows; got %d, expected 1", len(windows))
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, windows[0].ID, benchmarkParam.windowState); err != nil {
		s.Fatalf("Failed to set window state to %v: %v", benchmarkParam.windowState, err)
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})

	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddScreenshotRecorder(ctx, 0, 1); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	if benchmarkParam.benchmarkSetUp != nil {
		s.Logf("Setting up %s", benchmarkParam.name)
		if err := benchmarkParam.benchmarkSetUp(ctx, tconn); err != nil {
			s.Fatal("Failed to setup benchmark: ", err)
		}
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		s.Logf("Running %s", benchmarkParam.name)
		return benchmarkParam.benchmarkRun(ctx, benchmarkConn)
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	scores := make(map[string]float64)
	s.Logf("Retrieving %s scores", benchmarkParam.name)
	if err := benchmarkParam.benchmarkScore(ctx, benchmarkConn, scores); err != nil {
		s.Fatal("Failed to retrieve benchmark scores: ", err)
	}

	pv := perf.NewValues()
	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}

	for metric, value := range scores {
		pv.Set(perf.Metric{
			Name:      benchmarkPrefix + metric,
			Unit:      "score",
			Direction: perf.BiggerIsBetter,
		}, value)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
