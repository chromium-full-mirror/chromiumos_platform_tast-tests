// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package benchmarkcuj contains helper util and test code for BenchmarkCUJ.
package benchmarkcuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	benchmarkPrefix      = "Benchmark."
	imageCopyRepeatTimes = 150
)

// BenchmarkTest holds parameters for the BenchmarkCUJ test variants.
type BenchmarkTest struct {
	BrowserType   browser.Type
	BenchmarkInfo benchmarkInfo
	RecorderMode  cujrecorder.RecorderMode
	RunOnBattery  bool
	ImageSearch   bool
}

// Score holds values for a single metric along with their improvement direction.
type Score struct {
	unit      string
	direction perf.Direction
	values    []float64
}

// benchmarkInfo will contain benchmark specific information.
type benchmarkInfo struct {
	name           string
	windowState    ash.WindowStateType
	benchmarkURL   string
	benchmarkSetUp func(context.Context, *uiauto.Context) error
	benchmarkRun   func(context.Context, *chrome.Conn, *uiauto.Context, map[string]string) error
	benchmarkScore func(context.Context, *chrome.Conn, map[string]Score) error
	// Optional parameters for the benchmark.
	params []string
}

// Run runs the Benchmark CUJ by running the benchmark and recording the result.
func Run(ctx context.Context, s *testing.State) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

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

	// Make sure the benchmark page is the only window opened.
	if len(windows) != 1 {
		s.Fatalf("Unexpected number of windows; got %d, expected 1", len(windows))
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, windows[0].ID, benchmarkParam.windowState); err != nil {
		s.Fatalf("Failed to set window state to %v: %v", benchmarkParam.windowState, err)
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		Mode:              testParam.RecorderMode,
		CooldownBeforeRun: true,
		RunOnBattery:      testParam.RunOnBattery,
	})

	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddScreenshotRecorder(ctx, 0, 1); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	ac := uiauto.New(tconn)

	if benchmarkParam.benchmarkSetUp != nil {
		s.Logf("Setting up %s", benchmarkParam.name)
		if err := benchmarkParam.benchmarkSetUp(ctx, ac); err != nil {
			s.Fatal("Failed to setup benchmark: ", err)
		}
	}

	params := make(map[string]string)
	for _, param := range benchmarkParam.params {
		if val, ok := s.Var(iterationsVar); ok {
			params[param] = val
		}
	}

	if testParam.ImageSearch {
		// Get file base path.
		user := cr.NormalizedUser()
		testPicturePath := s.DataPath(launcher.ImageSearchPowerTestPictureName)
		cleanup, err := cuj.PrepareImageSearchFiles(ctx, user, testPicturePath, imageCopyRepeatTimes)
		if err != nil {
			s.Fatal("Failed to prepare image search files: ", err)
		}
		defer cleanup()

		// GoBigSleepLint: Wait to let the image indexing start.
		if err := testing.Sleep(ctx, 2*time.Minute); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		s.Logf("Running %s", benchmarkParam.name)
		return benchmarkParam.benchmarkRun(ctx, benchmarkConn, ac, params)
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	scores := make(map[string]Score)
	s.Logf("Retrieving %s scores", benchmarkParam.name)
	if err := benchmarkParam.benchmarkScore(ctx, benchmarkConn, scores); err != nil {
		s.Fatal("Failed to retrieve benchmark scores: ", err)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}

	for metric, score := range scores {
		direction := score.direction
		pv.Set(perf.Metric{
			Name:      benchmarkPrefix + metric,
			Unit:      score.unit,
			Direction: direction,
			Multiple:  len(score.values) > 1,
		}, score.values...)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
