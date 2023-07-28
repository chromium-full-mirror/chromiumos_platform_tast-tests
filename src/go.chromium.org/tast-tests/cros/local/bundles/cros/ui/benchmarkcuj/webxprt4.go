// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/errors"
)

const webxprt4Prefix = "WebXPRT4."

// WebXPRT4Info contains the information for running WebXPRT4 Benchmark.
var WebXPRT4Info = benchmarkInfo{
	name:           "WebXPRT4",
	windowState:    ash.WindowStateMaximized,
	benchmarkURL:   "http://www.principledtechnologies.com/benchmarkxprt/webxprt",
	benchmarkRun:   RunWebXPRT4,
	benchmarkScore: RetrieveWebXPRT4Score,
}

// RunWebXPRT4 runs the WebXPRT4 test.
func RunWebXPRT4(ctx context.Context, benchmarkConn *chrome.Conn, ac *uiauto.Context, params map[string]string) error {
	// Running WebXPRT4 is just navigating to the driver page so wait until it's over.
	if err := benchmarkConn.Navigate(ctx, `http://www.principledtechnologies.com/benchmarkxprt/webxprt/2021/wx4_build_3_7_3/auto.php?testtype=1&tests=63&result=2`); err != nil {
		return errors.Wrap(err, "failed to start WebXPRT4")
	}
	if err := benchmarkConn.WaitForExprWithTimeout(ctx, `document.getElementsByName('Overall Score').length == 1`, 30*time.Minute); err != nil {
		return errors.Wrap(err, "WebXPRT4 run timed out")
	}
	return nil
}

// RetrieveWebXPRT4Score retrieves the score after WebXPRT4 finished.
func RetrieveWebXPRT4Score(ctx context.Context, benchmarkConn *chrome.Conn, scores map[string]Score) error {
	// Map of test results.
	benchmarkScores := make(map[string]float64)
	// All the scores consist of a numeric value and a confidence interval.
	// The overall score is a BiggerIsBetter metric and its CI is in the same unit.
	// Particular tests are time metrics and their CI is in percent of the value.
	//
	// Code below gathers test values only until CI handling is finalized in
	// crosbolt.
	//
	// For more details see the Principled Technologies metric calculation
	// whitepaper:
	// https://www.principledtechnologies.com/benchmarkxprt/counter.php?inline=true&redirect=/benchmarkxprt/whitepapers/webxprt/WebXPRT-4-results-calculation.pdf
	if err := benchmarkConn.Eval(ctx, `
	new Promise(resolve => {
		function pascalCaser(match, p1, offset, string, groups) {
			return match[1].toUpperCase();
		}

		let scoreMap = new Map();
		let overallScore = document.getElementsByName("Overall Score");
		let testScores = document.getElementsByTagName("resptime");
		if (overallScore.length != 1) {
		    resolve(scoreMap);
		}

		let value = parseFloat(overallScore[0].textContent);
		scoreMap["Score"] = value;
		for (const test of testScores) {
		    // Strip '(ms)' and convert to PascalCase.
		    let name = 'test.' + test.attributes[0].value.replace(' (ms)','').trimEnd().replaceAll(/ [a-z]/ig, pascalCaser);
		    // Format is: <score> +/- <ci>%
			let valueParts = test.textContent.split(' ');
			let valueStr = valueParts[0];

			value = parseFloat(valueStr);
		    scoreMap[name] = value;
		}
		resolve(scoreMap);
	})`, &benchmarkScores); err != nil {
		return errors.Wrap(err, "failed to retrieve WebXPRT4 score")
	}

	if len(benchmarkScores) == 0 {
		return errors.New("WebXPRT4 crashed during the test")
	}
	for metric, score := range benchmarkScores {
		unit := "score"
		direction := perf.BiggerIsBetter
		if strings.HasPrefix(metric, `test.`) {
			unit = "ms"
			direction = perf.SmallerIsBetter
		}
		value := score

		scores[webxprt4Prefix+metric] = Score{unit, direction, []float64{value}}
	}
	return nil
}
