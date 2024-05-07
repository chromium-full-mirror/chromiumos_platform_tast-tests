// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bounds

import (
	"context"
	"os"
	"path"
	"regexp"
	"testing"
)

func setupDir(t *testing.T, testName, resultsChart string) string {
	baseDir := t.TempDir()
	outDir := path.Join(baseDir, testName)
	err := os.Mkdir(outDir, 0755)
	if err != nil {
		t.Fatalf("could not create temporary output directory")
	}
	err = os.WriteFile(path.Join(outDir, resultsChartFileName), []byte(resultsChart), 0666)
	if err != nil {
		t.Error("could not write results file to temp directory")
	}
	return outDir
}

const resultsChart = `
{
	"metric_A": {
		"summary": {
			"type": "list_of_scalar_values",
			"units": "sec",
			"values": [0.332915999999983, 0.297357999999974, 0.301710000000014],
			"improvement_direction": "down"
	  	}
	},
	"metric_B": {
	  	"summary": {
			"type": "scalar",
			"units": "sec",
			"value": 0.756493091583252,
			"improvement_direction": "down"
	  	}
	}
}
`

var exampleTest = MatchExact("example.Test")
var metricA = MatchExact("metric_A")
var metricB = MatchExact("metric_B")

func TestEvaluateResults(t *testing.T) {
	testCases := []struct {
		name         string
		testName     string
		metricBounds []MetricBounds
		wantPass     bool
		errRegexp    string
	}{{
		name:     "simple pass",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Test:   exampleTest,
			Metric: metricA,
			Bounds: Max(0.4),
		}, {
			Test:   exampleTest,
			Metric: metricB,
			Bounds: Min(0.7),
		}},
		wantPass: true,
	}, {
		name:     "simple fail",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Test:   exampleTest,
			Metric: metricB,
			Bounds: Max(0.2),
		}},
		wantPass:  false,
		errRegexp: `> upper bound`,
	}, {
		name:     "metric not matched",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Test:   exampleTest,
			Metric: MatchExact("nonexistent_metric"),
			Bounds: Max(0.5),
		}},
		wantPass:  false,
		errRegexp: `no metric found`,
	}, {
		name:     "some pass but some fail",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Test:   exampleTest,
			Metric: metricA,
			Bounds: Min(0.3),
		}},
		wantPass: false,
	}, {
		name:     "ignore metric for subtest",
		testName: "example.Test.subtest",
		metricBounds: []MetricBounds{{
			Test:   exampleTest,
			Metric: metricA,
			Bounds: Max(0.4),
		}, {
			Test:   MatchExact("example.Test.subtest"),
			Metric: metricB,
			Bounds: Min(0.7),
		}},
		wantPass: true,
	}, {
		name:     "failing when test name omitted",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Metric: metricB,
			Bounds: Max(0.2),
		}},
		wantPass: false,
	}, {
		name:     "passing when test name omitted",
		testName: "example.Test",
		metricBounds: []MetricBounds{{
			Metric: metricA,
			Bounds: Between(0.2, 0.4),
		}},
		wantPass: true,
	}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outDir := setupDir(t, tc.testName, resultsChart)
			gotErr := EvaluateResults(context.Background(), tc.metricBounds, outDir)
			if gotErr != nil && tc.wantPass {
				t.Errorf("EvaluateResults() returned error: %v", gotErr)
			}
			if gotErr == nil && !tc.wantPass {
				t.Errorf("EvaluateResults(): returned nil error, wanted non-nil error")
			}

			if gotErr != nil && tc.errRegexp != "" {
				r := regexp.MustCompile(tc.errRegexp)
				if !r.MatchString(gotErr.Error()) {
					t.Errorf("EvaluateResults(): wanted error to match %q, got: %v", tc.errRegexp, gotErr)
				}
			}
		})
	}
}

const missingValue = `
{
	"metric_A": {
		"summary": {
		}
	}
}
`

const missingSummary = `
{
	"metric_A": {

	},
	"metric_B": {

	}
}
`

func TestInvalidResultsChart(t *testing.T) {
	outDir := setupDir(t, "example.Test", missingValue)
	_, err := loadScalars(outDir)
	if err == nil { // if NO error
		t.Errorf("loadScalars(outDir): expected error due to missing \"value\"/\"values\" field")
	}
	outDir = setupDir(t, "example.Test", missingSummary)
	_, err = loadScalars(outDir)
	if err == nil { // if NO error
		t.Errorf("loadScalars(outDir): expected error due to missing \"summary\" field")
	}
}
