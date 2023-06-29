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

const krakenPrefix = "Kraken."

// KrakenInfo contains the information for running Kraken Benchmark.
var KrakenInfo = benchmarkInfo{
	name:           "Kraken",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "https://mozilla.github.io/krakenbenchmark.mozilla.org/",
	benchmarkRun:   RunKraken,
	benchmarkScore: RetrieveKrakenScore,
	unit:           "ms",
	direction:      perf.SmallerIsBetter,
}

// RunKraken runs the Kraken test.
func RunKraken(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	// Running Kraken is just navigating to the driver page
	if err := benchmarkConn.Navigate(ctx, "https://mozilla.github.io/krakenbenchmark.mozilla.org/kraken-1.1/driver"); err != nil {
		return errors.Wrap(err, "failed to start Kraken")
	}
	resultPage := nodewith.NameContaining("Kraken JavaScript Benchmark Results").Role(role.Window).ClassName("Widget")
	if err := ac.WithInterval(5 * time.Second).WithTimeout(15 * time.Minute).WaitUntilExists(resultPage)(ctx); err != nil {
		return errors.Wrap(err, "failed to run Kraken")
	}
	return nil
}

// RetrieveKrakenScore retrieves the score after Kraken finished.
func RetrieveKrakenScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string][]float64) error {
	benchmarkScores := make(map[string]float64)
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		let scoreMap = new Map();
		if (!mean || !categoryMeans || !testMeansByCategory) {
			resolve(scoreMap);
		}
		scoreMap["Score"] = mean;
		for (const [category, cValue] of Object.entries(categoryMeans)) {
			scoreMap[category] = cValue;
			for (const [subCategory, value] of Object.entries(testMeansByCategory[category])) {
				scoreMap[subCategory.replace(/-/g,"_")] = value;
			}
		}
		resolve(scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve Kraken scores")
	}
	if len(benchmarkScores) == 0 {
		return errors.New("Kraken crashed during the test")
	}
	for metric, value := range benchmarkScores {
		scores[krakenPrefix+metric] = []float64{value}
	}
	return nil
}
