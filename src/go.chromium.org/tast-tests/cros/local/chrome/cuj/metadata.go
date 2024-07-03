// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type feature string

const (
	batterySaver   feature = "battery_saver"
	arcDisabled    feature = "arc_disabled"
	pvSched        feature = "pvsched"
	fieldTrials    feature = "field_trials"
	roundedWindows feature = "rounded_windows"
	vulkan         feature = "vulkan"
	wprFeature     feature = "wpr"
)

// Metadata represents metadata for a performance CUJ or a performance test.
type Metadata struct {
	DisplayName  string    // Optional display name to be shown on the dashboard.
	BaseTestName string    // Optional base test that this should be compared to.
	Metrics      []string  // Recommended metrics for this test.
	Features     []feature // Features that this test is testing.
}

var defaultMetrics = []string{
	"TPS.Power.Timeline",
	"Ash.Smoothness.PercentDroppedFrames_1sWindow2",
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
	"EventLatency.MousePressed.TotalLatency",
	"EventLatency.KeyPressed.TotalLatency",
	"EventLatency.TotalLatency",
	"Ash.EventLatency.TotalLatency",
	"PageLoad.PaintTiming.NavigationToFirstContentfulPaint",
	"PageLoad.PaintTiming.NavigationToLargestContentfulPaint2",
}

// Benchmark metrics.
const (
	speedometer  = "Benchmark.Speedometer.Score"
	speedometer3 = "Benchmark.Speedometer3.Score"
	motionmark   = "Benchmark.Motionmark.Score"
	kraken       = "Benchmark.Kraken.Score"
	octane       = "Benchmark.Octane.Score"
	jetstream    = "Benchmark.Jetstream.Score"
	webxprt4     = "Benchmark.WebXPRT4.Score"
)

var registry = map[string]Metadata{
	"ui.DesksCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DesksCUJ.pvsched": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{pvSched},
		Metrics:      defaultMetrics,
	},
	"ui.DesksCUJ.field_trials": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{fieldTrials},
		Metrics:      defaultMetrics,
	},
	"ui.DesksCUJ.battery_saver": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{batterySaver},
		Metrics:      defaultMetrics,
	},
	"ui.DesksCUJ.rounded_windows": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{roundedWindows},
		Metrics:      defaultMetrics,
	},
	"ui.DesksCUJ.vulkan": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{vulkan},
		Metrics:      defaultMetrics,
	},
	"ui.DesksCUJ.arc_disabled": Metadata{
		BaseTestName: "ui.DesksCUJ",
		Features:     []feature{arcDisabled},
		Metrics:      defaultMetrics,
	},
	"ui.BenchmarkCUJ.speedometer": Metadata{
		Metrics: []string{speedometer},
	},
	"ui.BenchmarkCUJ.speedometer3": Metadata{
		Metrics: []string{speedometer3},
	},
	"ui.BenchmarkCUJ.motionmark": Metadata{
		Metrics: []string{motionmark},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark": Metadata{
		Features: []feature{vulkan},
		Metrics:  []string{motionmark},
	},
	"ui.BenchmarkCUJ.motionmark1_3": Metadata{
		DisplayName:  "BenchmarkMotionmark 1.3",
		BaseTestName: "ui.BenchmarkCUJ.motionmark",
		Metrics:      []string{motionmark},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark1_3": Metadata{
		DisplayName:  "BenchmarkMotionmarkVulkan 1.3",
		BaseTestName: "ui.BenchmarkCUJ.vulkan_motionmark",
		Features:     []feature{vulkan},
		Metrics:      []string{motionmark},
	},
	"ui.BenchmarkCUJ.jetstream": Metadata{
		Metrics: []string{jetstream},
	},
	"ui.BenchmarkCUJ.kraken": Metadata{
		Metrics: []string{kraken},
	},
	"ui.BenchmarkCUJ.octane": Metadata{
		Metrics: []string{octane},
	},
	"ui.BenchmarkCUJ.webxprt4": Metadata{
		Metrics: []string{webxprt4},
	},
	"ui.BenchmarkCUJ.vulkan_webxprt4": Metadata{
		Features: []feature{vulkan},
		Metrics:  []string{webxprt4},
	},
	"ui.BenchmarkCUJ.speedometer_wpr": Metadata{
		BaseTestName: "ui.BenchmarkCUJ.speedometer",
		Features:     []feature{wprFeature},
		Metrics:      []string{speedometer},
	},
	"ui.BenchmarkCUJ.motionmark_wpr": Metadata{
		BaseTestName: "ui.BenchmarkCUJ.motionmark",
		Features:     []feature{wprFeature},
		Metrics:      []string{motionmark},
	},
	"ui.BenchmarkCUJ.kraken_wpr": Metadata{
		BaseTestName: "ui.BenchmarkCUJ.kraken",
		Features:     []feature{wprFeature},
		Metrics:      []string{kraken},
	},
	"ui.BenchmarkCUJ.octane_wpr": Metadata{
		BaseTestName: "ui.BenchmarkCUJ.octane",
		Features:     []feature{wprFeature},
		Metrics:      []string{octane},
	},
}

// GenerateMetadataFile stores a metadata.json file in the testing out
// directory representing |registry|.
func GenerateMetadataFile(ctx context.Context) error {
	// Perform a simple initial check that the base names for each test are
	// valid, by ensuring that the referenced base name has its own metadata.
	for _, test := range registry {
		if test.BaseTestName == "" {
			continue
		}

		if _, ok := registry[test.BaseTestName]; !ok {
			return errors.Errorf("found invalid test metadata, base test %s doesn't exist", test.BaseTestName)
		}
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return errors.New("failed to get the out directory")
	}

	json, err := json.MarshalIndent(&registry, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshal metadata %v", registry)
	}

	if err := os.WriteFile(filepath.Join(outDir, "metadata.json"), json, 0644); err != nil {
		return errors.Wrap(err, "failed to write metadata")
	}

	return nil
}
