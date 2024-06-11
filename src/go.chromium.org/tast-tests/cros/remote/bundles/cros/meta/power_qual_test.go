// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	_ "embed"
	"os"
	"path"
	"testing"

	"go.chromium.org/tast-tests/cros/common/bounds"
)

// An actual results-chart.json from a run of meta.PowerQual.qual
//
//go:embed testdata/results-chart.json
var resultsChart string

func TestPowerQualBounds(t *testing.T) {
	testCases := []string{
		"qual",
		"browsing",
		"browsingheavy",
		"videoplayback",
		"videocall",
	}
	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			tempDir := t.TempDir()
			outDir := path.Join(tempDir, "meta.PowerQual."+tc)
			if err := os.Mkdir(outDir, 0755); err != nil {
				t.Fatalf("failed to create temp dir: %s", err)
			}
			if err := os.WriteFile(path.Join(outDir, "results-chart.json"), []byte(resultsChart), 0666); err != nil {
				t.Fatalf("failed to write results-chart.json to temp dir: %s", err)
			}
			if err := bounds.EvaluateResults(context.Background(), powerQualBounds, outDir); err != nil {
				t.Fatalf("EvaluateResults() returned error: %s", err)
			}
		})
	}
}
