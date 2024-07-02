// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	_ "embed"
	"os"
	"path"
	"testing"

	"go.chromium.org/tast-tests/cros/common/bounds"
)

// A results-chart.json from a run of power.LowPowerConsumption.shutdown
//
//go:embed testdata/lpc-shutdown-results-chart.json
var shutdownResultsChart string

// A results-chart.json from a run of power.LowPowerConsumption.suspend
//
//go:embed testdata/lpc-suspend-results-chart.json
var suspendResultsChart string

func TestPowerQualBounds(t *testing.T) {
	testCases := []struct {
		Name         string
		ResultsChart string
	}{{
		Name:         "shutdown",
		ResultsChart: shutdownResultsChart,
	}, {
		Name:         "suspend",
		ResultsChart: suspendResultsChart,
	}}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			tempDir := t.TempDir()
			outDir := path.Join(tempDir, "power.LowPowerConsumption."+tc.Name)
			if err := os.Mkdir(outDir, 0755); err != nil {
				t.Fatalf("failed to create temp dir: %s", err)
			}
			if err := os.WriteFile(path.Join(outDir, "results-chart.json"), []byte(tc.ResultsChart), 0666); err != nil {
				t.Fatalf("failed to write results-chart.json to temp dir: %s", err)
			}
			if err := bounds.EvaluateResults(context.Background(), lpcMetricBounds, outDir); err != nil {
				t.Fatalf("EvaluateResults() returned error: %s", err)
			}
		})
	}
}
