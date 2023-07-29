// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/errors"
)

const octanePrefix = "Octane."

// OctaneInfo contains the information for running Octane Benchmark.
var OctaneInfo = benchmarkInfo{
	name:           "Octane",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "http://chromium.github.io/octane/",
	benchmarkRun:   RunOctane,
	benchmarkScore: RetrieveOctaneScore,
}

// RunOctane runs the Octane test.
func RunOctane(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		BenchmarkSuite.scoreMap = new Map();
		// Overwrite these functions to store the scores.
		originalAddScore = AddScore;
		AddScore = function(score) {
			originalAddScore(score);
			BenchmarkSuite.scoreMap["Score"] = parseInt(score);
			resolve();
		}
		originalAddResult = AddResult;
		AddResult = function(name, result) {
			originalAddResult(name, result);
			BenchmarkSuite.scoreMap[name] = parseInt(result);
		}
		Run();
	})`, nil); err != nil {
		return errors.Wrap(err, "failed to run Octane")
	}
	return nil
}

// RetrieveOctaneScore retrieves the score after Octane finished.
func RetrieveOctaneScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string]Score) error {
	benchmarkScores := make(map[string]float64)
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		if (!BenchmarkSuite.scoreMap) {
			resolve(new Map());
		}
		resolve(BenchmarkSuite.scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve Octane score")
	}

	if len(benchmarkScores) == 0 {
		return errors.New("Octane crashed during the test")
	}
	for metric, value := range benchmarkScores {
		scores[octanePrefix+metric] = Score{"score", perf.BiggerIsBetter, []float64{value}}
	}
	return nil
}
