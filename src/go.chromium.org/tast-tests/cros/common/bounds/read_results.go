// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bounds

import (
	"encoding/json"
	"os"
	"path"

	"go.chromium.org/tast/core/errors"
)

type scalar struct {
	name  string
	value float64
}

const resultsChartFileName = "results-chart.json"

func loadScalars(outDir string) ([]scalar, error) {
	b, err := os.ReadFile(path.Join(outDir, resultsChartFileName))
	if err != nil {
		return nil, errors.Wrap(err, "load results chart")
	}
	// The "summary" field of a metric in results-chart.json
	// will either have a "value" key associated with a float
	// or a "values" key associated with an array of floats.
	type ResultsChart map[string]struct {
		Summary struct {
			Value  *float64  `json:"value"`
			Values []float64 `json:"values"`
		} `json:"summary"`
	}
	var resultsChart ResultsChart
	err = json.Unmarshal(b, &resultsChart)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal results chart")
	}
	var out []scalar
	for k, v := range resultsChart {
		switch {
		case v.Summary.Value != nil:
			out = append(out, scalar{name: k, value: *v.Summary.Value})
		case v.Summary.Values != nil:
			for _, value := range v.Summary.Values {
				out = append(out, scalar{name: k, value: value})
			}
		default:
			return nil, errors.Errorf(`unmarshal results chart: neither "value" nor "values" field present in summary of metric %q`, k)
		}
	}
	return out, nil
}
