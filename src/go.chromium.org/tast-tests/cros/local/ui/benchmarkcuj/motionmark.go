// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const motionMarkPrefix = "MotionMark."

// MotionMarkInfo contains the information for running MotionMark Benchmark.
var MotionMarkInfo = benchmarkInfo{
	name:           "MotionMark",
	windowState:    ash.WindowStateFullscreen,
	benchmarkURL:   "https://browserbench.org/MotionMark1.2/",
	benchmarkRun:   RunMotionMark,
	benchmarkScore: RetrieveMotionMarkScore,
}

// MotionMark1_3Info contains the information for running MotionMark 1.3 Benchmark.
var MotionMark1_3Info = benchmarkInfo{
	name:           "MotionMark1.3",
	windowState:    ash.WindowStateFullscreen,
	benchmarkURL:   "https://browserbench.org/MotionMark1.3/",
	benchmarkSetUp: SetUpMotionMark,
	benchmarkRun:   RunMotionMark,
	benchmarkScore: RetrieveMotionMarkScore,
}

// SetUpMotionMark makes sure the test is ready to run.
func SetUpMotionMark(ctx context.Context, ac *uiauto.Context) error {

	// Wait for up to 2 minutes for MotionMark to detect framerate.
	const detectWaitTime = 2 * time.Minute

	framerateText := nodewith.NameContaining("Framerate").Role(role.StaticText)
	if err := ac.WithTimeout(detectWaitTime).WaitUntilExists(framerateText)(ctx); err != nil {
		return errors.Wrap(err, "benchmark failed to detect framerate")
	}

	return nil
}

// RunMotionMark runs the MotionMark test.
func RunMotionMark(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		benchmarkRunnerClient.didFinishLastIteration = function() {
			benchmarkController.showResults();
			resolve();
		};
		benchmarkController.startBenchmark();
	})`, nil); err != nil {
		return errors.Wrap(err, "failed to run MotionMark")
	}
	return nil
}

// RetrieveMotionMarkScore retrieves the score after MotionMark finished.
func RetrieveMotionMarkScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string]Score) error {
	benchmarkScores := make(map[string]float64)
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		let scoreMap = new Map();
		if (!benchmarkRunnerClient || !benchmarkRunnerClient.results
			|| !benchmarkRunnerClient.results.results) {
			resolve(scoreMap);
		}

		results = benchmarkRunnerClient.results.results[0];
		testResults = results.testsResults.MotionMark;

		scoreMap["Score"] = results.score;
		for (const [key, value] of Object.entries(testResults)) {
			if (key) {
				scoreMap[key.replace(/ /g,"_")] = value.score;
			}
		}
		resolve(scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve MotionMark scores")
	}
	if len(benchmarkScores) == 0 {
		return errors.New("MotionMark crashed during the test")
	}
	for metric, value := range benchmarkScores {
		if value < 10 {
			testing.ContextLogf(ctx, "MotionMark %s subtest score below 10; got a score of %f", metric, value)
		}
		scores[motionMarkPrefix+metric] = Score{"score", perf.BiggerIsBetter, []float64{value}}
	}
	return nil
}
