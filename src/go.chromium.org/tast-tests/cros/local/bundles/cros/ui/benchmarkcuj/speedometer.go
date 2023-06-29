// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"context"
	"fmt"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	iterationsVar     = "iterations"
	speedometerPrefix = "Speedometer."
)

// SpeedometerInfo contains the information for running Speedometer Benchmark.
var SpeedometerInfo = benchmarkInfo{
	name:           "Speedometer",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "https://browserbench.org/Speedometer2.1/",
	benchmarkRun:   RunSpeedometer,
	benchmarkScore: RetrieveSpeedometerScore,
	unit:           "runs-per-min",
	direction:      perf.BiggerIsBetter,
	params:         []string{iterationsVar},
}

// RunSpeedometer runs the Speedometer test.
func RunSpeedometer(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	// Speedometer benchmark defaults with 10 iterations.
	iter := 10
	var err error

	iterationStr, ok := params[iterationsVar]
	if ok {
		iter, err = strconv.Atoi(iterationStr)
		if err != nil {
			return errors.New("iteration variable is not an integer")
		}
	}

	if err := benchmarkConn.Eval(ctx, fmt.Sprintf(`
	new Promise(resolve => {
		benchmarkClient.iterationCount = %d;
		// Overwrite this function to include the resolve statement at the end.
		benchmarkClient.originalLastFunction = benchmarkClient.didFinishLastIteration;
		benchmarkClient.didFinishLastIteration = function() {
			benchmarkClient.originalLastFunction();
			resolve();
		};
		startTest();
	})`, iter), nil); err != nil {
		return errors.Wrap(err, "failed to run Speedometer")
	}
	return nil
}

// RetrieveSpeedometerScore retrieves the score after Speedometer finished.
func RetrieveSpeedometerScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string][]float64) error {
	benchmarkScores := make(map[string][]float64)
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		let scoreMap = new Map();
		if (!benchmarkClient || benchmarkClient._measuredValuesList.length <= 0 ||
			benchmarkClient._measuredValuesList.length !== benchmarkClient.iterationCount) {
				scoreMap["Score"] = [-1]
			resolve(scoreMap);
		}
		let total = 0;
		let scoreList = [];
		for (const result of benchmarkClient._measuredValuesList) {
			scoreList.push(result.score);
			total += result.score;
		}
		scoreMap["Score"] = [total / benchmarkClient.iterationCount];
		scoreMap["Runs_Scores"] = scoreList;
		resolve(scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve Speedometer score")
	}

	if len(benchmarkScores["Score"]) == 1 && benchmarkScores["Score"][0] < 0 {
		return errors.New("Speedometer crashed during the test")
	}

	for metric, value := range benchmarkScores {
		scores[speedometerPrefix+metric] = value
	}

	mean, sd, delta, err := getDelta(benchmarkScores["Runs_Scores"])
	if err != nil {
		return errors.Wrap(err, "failed to calculate confidence interval")
	}

	scores[speedometerPrefix+"sd"] = []float64{sd}
	scores[speedometerPrefix+"95_percent_ci"] = []float64{mean - delta, mean + delta}

	testing.ContextLogf(ctx, "Speedometer Mean: %.2f", mean)
	testing.ContextLogf(ctx, "Speedometer Standard Deviation: %.2f", sd)
	testing.ContextLog(ctx, "Speedometer Scores: ", benchmarkScores["Runs_Scores"])
	testing.ContextLogf(ctx, "Speedometer 95 percent confidence interval [%.2f, %.2f]", mean-delta, mean+delta)

	return nil
}
