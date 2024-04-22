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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
	SkipCooldown  bool
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
func Run(ctx context.Context, cr *chrome.Chrome, testParam BenchmarkTest, cmdLineArgs func(string) (string, bool)) (pv *perf.Values, retErr error) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	benchmarkParam := testParam.BenchmarkInfo

	benchmarkConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr,
		testParam.BrowserType, benchmarkParam.benchmarkURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to setup Chrome with %s", benchmarkParam.benchmarkURL)
	}
	defer closeBrowser(closeCtx)
	defer benchmarkConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the test API connection")
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "falied to connect to browser test API connection")
	}

	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return nil, errors.New("failed to get the out directory")
	}
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, dir, func() bool { return retErr != nil }, cr, "ui_dump")

	// Set window to benchmark's preferred window state.
	windows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all windows")
	}

	// Make sure the benchmark page is the only window opened.
	if len(windows) != 1 {
		return nil, errors.Errorf("unexpected number of windows; got %d, expected 1", len(windows))
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, windows[0].ID, benchmarkParam.windowState); err != nil {
		return nil, errors.Wrapf(err, "failed to set window state to %v", benchmarkParam.windowState)
	}

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{
		Mode:              testParam.RecorderMode,
		CooldownBeforeRun: !testParam.SkipCooldown,
		RunOnBattery:      testParam.RunOnBattery,
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddScreenshotRecorder(ctx, 0, 1); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}

	ac := uiauto.New(tconn)

	if benchmarkParam.benchmarkSetUp != nil {
		testing.ContextLogf(ctx, "Setting up %s", benchmarkParam.name)
		if err := benchmarkParam.benchmarkSetUp(ctx, ac); err != nil {
			return nil, errors.Wrap(err, "failed to setup benchmark")
		}
	}

	params := make(map[string]string)
	for _, param := range benchmarkParam.params {
		if val, ok := cmdLineArgs(iterationsVar); ok {
			params[param] = val
		}
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		pv, err = localPerf.CaptureDeviceSnapshot(ctx, "Initial")
		if err != nil {
			return errors.Wrap(err, "failed to capture device snapshot")
		}

		testing.ContextLogf(ctx, "Running %s", benchmarkParam.name)
		return benchmarkParam.benchmarkRun(ctx, benchmarkConn, ac, params)
	}); err != nil {
		return nil, errors.Wrap(err, "failed to conduct the recorder task")
	}

	scores := make(map[string]Score)
	testing.ContextLogf(ctx, "Retrieving %s scores", benchmarkParam.name)
	if err := benchmarkParam.benchmarkScore(ctx, benchmarkConn, scores); err != nil {
		return nil, errors.Wrap(err, "failed to retrieve benchmark scores")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return nil, errors.Wrap(err, "failed to report")
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

	return pv, nil
}
