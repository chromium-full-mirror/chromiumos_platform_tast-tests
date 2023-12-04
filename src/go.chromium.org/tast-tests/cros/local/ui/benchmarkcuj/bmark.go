// Copyright 2024 The ChromiumOS Authors
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

const bmarkPrefix = "Bmark."

// BmarkInfo contains the information for running Bmark Benchmark.
var BmarkInfo = benchmarkInfo{
	name:           "Bmark",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "https://www.wirple.com/bmark/",
	benchmarkRun:   RunBmark,
	benchmarkScore: RetrieveBmarkScore,
}

// RunBmark runs the Bmark test.
func RunBmark(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	if err := benchmarkConn.Eval(ctx, `
		new Promise(resolve => {
			$('.starttest').click();

			function didFinishLastIteration() {
				if (document.querySelector('.score').textContent.includes('Total score:')) {
					resolve();
				} else {
					setTimeout(didFinishLastIteration, 1000); // Check every second
				}
			}

			didFinishLastIteration();
		})`, nil); err != nil {
		return errors.Wrap(err, "failed to run Bmark")
	}
	return nil
}

// RetrieveBmarkScore retrieves the score after Bmark finished.
func RetrieveBmarkScore(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string]Score) error {
	benchmarkScores := make(map[string]float64)
	if err := benchmarkConn.Eval(ctx, `
		new Promise(resolve => {
			let scoreMap = new Map();
			let scoreElements = document.querySelectorAll('.score p');

			scoreElements.forEach(function(p) {
				let line = p.textContent;

				if (line.includes('Canvas score')) {
					let canvasScores = line.match(/Canvas score - Test 1: (\d+) - Test 2: (\d+)/);
					if (canvasScores && canvasScores.length === 3) {
						scoreMap['CanvasScoreTest1'] = parseFloat(canvasScores[1]);
						scoreMap['CanvasScoreTest2'] = parseFloat(canvasScores[2]);
					}
					return;
				}
				if (line.includes('WebGL score')) {
					let webGLScores = line.match(/WebGL score - Test 1: (\d+) - Test 2: (\d+)/);
					if (webGLScores && webGLScores.length === 3) {
						scoreMap['WebGLScoreTest1'] = parseFloat(webGLScores[1]);
						scoreMap['WebGLScoreTest2'] = parseFloat(webGLScores[2]);
					}
					return;
				}
				if (line.includes('Total score')) {
					let totalScore = line.match(/Total score: (\d+)/);
					if (totalScore && totalScore.length === 2) {
						scoreMap['TotalScore'] = parseFloat(totalScore[1]);
					}
				}
			});
			resolve(scoreMap);
		})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve Bmark scores")
	}

	expectedBenchmarkScores := 5
	if len(benchmarkScores) != expectedBenchmarkScores {
		return errors.Errorf("incomplete benchmark scores: got %d scores; want %d scores. Bmark may have crashed during the test", len(benchmarkScores), expectedBenchmarkScores)
	}
	for metric, value := range benchmarkScores {
		scores[bmarkPrefix+metric] = Score{"score", perf.BiggerIsBetter, []float64{value}}
	}
	return nil
}
