// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	_ "embed"
	"os"
	"path"
	"testing"

	"go.chromium.org/tast-tests/cros/common/bounds"
	"go.chromium.org/tast/core/framework/protocol"
	tastTesting "go.chromium.org/tast/core/testing"
)

// A results-chart.json from a run of power.LowPowerConsumption.shutdown
//
//go:embed testdata/boot-perf-results-chart.json
var resultsChart string

func TestPowerQualBounds(t *testing.T) {
	err := tastTesting.InitializeVarsForUnitTest(nil)
	if err != nil {
		t.Fatal("Unexpeted error: ", err)
	}
	tempDir := t.TempDir()
	outDir := path.Join(tempDir, "platform.BootPerf.default_bounds")
	if err := os.Mkdir(outDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %s", err)
	}
	if err := os.WriteFile(path.Join(outDir, "results-chart.json"), []byte(resultsChart), 0666); err != nil {
		t.Fatalf("failed to write results-chart.json to temp dir: %s", err)
	}
	bootPerfMetricBounds, err := bootPerfMetricBounds(context.Background(), &protocol.DUTFeatures{}, "someboard", nil)
	if err != nil {
		t.Fatal("Unexpeted error: ", err)
	}
	if err := bounds.EvaluateResults(context.Background(), bootPerfMetricBounds, outDir); err != nil {
		t.Fatalf("EvaluateResults() returned error: %s", err)
	}
}
