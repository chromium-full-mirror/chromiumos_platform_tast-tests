// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
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
}

// RunSpeedometer runs the Speedometer test.
func RunSpeedometer(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context) error {
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		// Overwrite this function to include the resolve statement at the end.
		benchmarkClient.originalLastFunction = benchmarkClient.didFinishLastIteration;
		benchmarkClient.didFinishLastIteration = function() {
			benchmarkClient.originalLastFunction();
			resolve();
		};
		startTest();
	})`, nil); err != nil {
		return errors.Wrap(err, "failed to run Speedometer")
	}
	return nil
}

// RetrieveSpeedometerScore retrieves the score after Speedometer finished.
func RetrieveSpeedometerScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string]float64) error {
	var score float64
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		if (!benchmarkClient || benchmarkClient._measuredValuesList.length <= 0 ||
			benchmarkClient._measuredValuesList.length !== benchmarkClient.iterationCount) {
			resolve(-1.0);
		}
		let total = 0;
		for (const result of benchmarkClient._measuredValuesList) {
			total += result.score;
		}
		resolve(total / benchmarkClient.iterationCount);
	})`, &score); err != nil {
		return errors.Wrap(err, "failed to retrieve Speedometer score")
	}

	if score < 0 {
		return errors.New("Speedometer crashed during the test")
	}
	scores["Speedometer.Score"] = score
	return nil
}
