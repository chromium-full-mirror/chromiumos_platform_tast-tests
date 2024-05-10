// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/bounds"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Bounds,
		Desc:         "Demonstrates how to set bounds on perf metrics",
		Contacts:     []string{"chromeos-pvs-eng@google.com", "weingartner@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Params: []testing.Param{{
			Name: "case_1",
			// The values here represent the performance results that will be recorded for the test.
			Val: map[string]float64{
				"example_metric_1": 2.6,
				"example_metric_2": 60,
			},
		}, {
			Name: "case_2",
			Val: map[string]float64{
				"example_metric_1": 2.7,
				"example_metric_2": 60,
			},
		}, {
			Name: "case_3",
			Val: map[string]float64{
				"example_metric_1": 2.8,
				"example_metric_2": 60,
			},
		}, {
			Name: "less_strict",
			Val: map[string]float64{
				"example_metric_1": 3.2,
				"example_metric_2": 60,
			},
		}, {
			Name: "missing",
			Val: map[string]float64{
				"example_metric_1": 2.5,
			},
		}, {
			Name: "out_of_bounds",
			Val: map[string]float64{
				"example_metric_1": 2.5,
				"example_metric_2": 59,
			},
		}},
	})
}

func Bounds(ctx context.Context, s *testing.State) {
	// See example.Perf for info on performance metrics.

	exampleBounds := []bounds.MetricBounds{{
		// This bound applies to "example.Bounds.case_1", "example.Bounds.case_2", and "example.Bounds.case_3",
		// but not the other cases.
		Test:   bounds.MatchRegexp(`case`),
		Metric: bounds.MatchExact("example_metric_1"),
		Bounds: bounds.Max(3.0),
	}, {
		// This bound applies only to "example.Bounds.less_strict". This shows how parameters
		// can have different acceptable ranges for the same performance metrics.
		Test:   bounds.MatchExact("example.Bounds.less_strict"),
		Metric: bounds.MatchExact("example_metric_1"),
		Bounds: bounds.Max(3.5),
	}, {
		// This bound omits the Test field, meaning that EvaluateResults() always applies this bound
		// regardless of the name of the test.
		Metric: bounds.MatchExact("example_metric_2"),
		Bounds: bounds.Equals(60),
	}}

	perfVals := s.Param().(map[string]float64)
	p := perf.NewValues()
	for name, val := range perfVals {
		p.Set(perf.Metric{Name: name, Unit: "s"}, val)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}

	// Only call EvaluateResults() after performance values have been saved to disk.
	if err := bounds.EvaluateResults(ctx, exampleBounds, s.OutDir()); err != nil {
		s.Error("Failed bounds check: ", err)
	}
}
