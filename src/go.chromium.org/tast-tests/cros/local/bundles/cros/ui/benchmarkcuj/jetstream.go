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
)

const jetStreamPrefix = "JetStream."

// JetStreamInfo contains the information for running JetStream Benchmark.
var JetStreamInfo = benchmarkInfo{
	name:           "JetStream",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "https://browserbench.org/JetStream/",
	benchmarkSetUp: SetUpJetStream,
	benchmarkRun:   RunJetStream,
	benchmarkScore: RetrieveJetStreamScore,
	unit:           "score",
	direction:      perf.BiggerIsBetter,
}

// SetUpJetStream prepares the start state for Jetstream to run.
func SetUpJetStream(ctx context.Context, ac *uiauto.Context) error {
	// Wait for up to 2 minutes for JetStream to load resources.
	startButton := nodewith.Name("Start Test").ClassName("button").Role(role.Link)
	if err := ac.WithTimeout(2 * time.Minute).WaitUntilExists(startButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to load JetStream resources")
	}
	return nil
}

// RunJetStream runs the JetStream test.
func RunJetStream(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	if err := benchmarkConn.Eval(ctx, `
		JetStream.start();
	`, nil); err != nil {
		return errors.Wrap(err, "failed to run JetStream")
	}
	return nil
}

// RetrieveJetStreamScore retrieves the score after JetStream finished.
func RetrieveJetStreamScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string][]float64) error {
	benchmarkScores := make(map[string]float64)
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		let scoreMap = new Map();
		let scores = [];
		for (const b of JetStream.benchmarks) {
			// If not all sub-tests finished, test crashed.
			if (!b.hasOwnProperty("endTime")) {
				resolve(new Map());
			}
			scoreMap[b.name.replace(/-/g,"_")] = b.score;
			scores.push(b.score);
		}
		// geomean is a built-in function for final score.
		scoreMap["Score"] = geomean(scores);
		resolve(scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve JetStream scores")
	}
	if len(benchmarkScores) == 0 {
		return errors.New("JetStream crashed during the test")
	}
	for metric, value := range benchmarkScores {
		scores[jetStreamPrefix+metric] = []float64{value}
	}
	return nil
}
