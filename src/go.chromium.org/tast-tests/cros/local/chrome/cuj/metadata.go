// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

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
	chromevox      feature = "chromevox"
	imageIndexing  feature = "image_indexing"
)

// Metadata represents metadata for a performance CUJ or a performance test.
type Metadata struct {
	// Test name associated with the given metadata. This field will be set
	// automatically when using WriteMetadataFile.
	TestName string

	DisplayName   string    // Optional display name to be shown on the dashboard.
	BaseTestNames []string  // Optional base tests that this test could be compared to.
	Metrics       []string  // Recommended metrics for this test.
	Features      []feature // Features that this test is testing.
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

// VideoCUJ metrics.
var videoMetrics = append(defaultMetrics, []string{
	"CrosVideo.DroppedFrames",
	"CrosVideo.PercentDroppedFrames",
}...)

// Benchmark metrics.
const (
	speedometerMetric  = "Benchmark.Speedometer.Score"
	speedometer3Metric = "Benchmark.Speedometer3.Score"
	motionmarkMetric   = "Benchmark.Motionmark.Score"
	krakenMetric       = "Benchmark.Kraken.Score"
	octaneMetric       = "Benchmark.Octane.Score"
	jetstreamMetric    = "Benchmark.Jetstream.Score"
	webxprt4Metric     = "Benchmark.WebXPRT4.Score"
)

// Registry maps test name to its corresponding metadata.
var Registry = map[string]Metadata{
	"ui.DesksCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DesksCUJ.pvsched": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{pvSched},
	},
	"ui.DesksCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.DesksCUJ.battery_saver": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{batterySaver},
	},
	"ui.DesksCUJ.rounded_windows": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{roundedWindows},
	},
	"ui.DesksCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.DesksCUJ.arc_disabled": Metadata{
		BaseTestNames: []string{"ui.DesksCUJ"},
		Features:      []feature{arcDisabled},
	},
	"ui.BenchmarkCUJ.speedometer": Metadata{
		Metrics: []string{speedometerMetric},
	},
	"ui.BenchmarkCUJ.speedometer3": Metadata{
		Metrics: []string{speedometer3Metric},
	},
	"ui.BenchmarkCUJ.motionmark": Metadata{
		Metrics: []string{motionmarkMetric},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.motionmark"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.motionmark1_3": Metadata{
		DisplayName:   "BenchmarkMotionmark 1.3",
		BaseTestNames: []string{"ui.BenchmarkCUJ.motionmark"},
	},
	"ui.BenchmarkCUJ.vulkan_motionmark1_3": Metadata{
		DisplayName:   "BenchmarkMotionmarkVulkan 1.3",
		BaseTestNames: []string{"ui.BenchmarkCUJ.vulkan_motionmark"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.jetstream": Metadata{
		Metrics: []string{jetstreamMetric},
	},
	"ui.BenchmarkCUJ.kraken": Metadata{
		Metrics: []string{krakenMetric},
	},
	"ui.BenchmarkCUJ.octane": Metadata{
		Metrics: []string{octaneMetric},
	},
	"ui.BenchmarkCUJ.webxprt4": Metadata{
		Metrics: []string{webxprt4Metric},
	},
	"ui.BenchmarkCUJ.vulkan_webxprt4": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.webxprt4"},
		Features:      []feature{vulkan},
	},
	"ui.BenchmarkCUJ.speedometer_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.speedometer"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.motionmark_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.motionmark"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.kraken_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.kraken"},
		Features:      []feature{wprFeature},
	},
	"ui.BenchmarkCUJ.octane_wpr": Metadata{
		BaseTestNames: []string{"ui.BenchmarkCUJ.octane"},
		Features:      []feature{wprFeature},
	},
	"ui.GoogleSheetsCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.GoogleSheetsCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.GoogleSheetsCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.TabSwitchPerf": Metadata{
		Metrics: []string{
			"Chrome.Tabs.AnimationSmoothness.TabLoading",
			"Browser.Tabs.TotalSwitchDuration3.WithSavedFrames",
			"Browser.Tabs.TotalSwitchDuration",
			"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
		},
	},
	"ui.TaskSwitchCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.TaskSwitchCUJ.tablet": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.TaskSwitchCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.TaskSwitchCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.TaskSwitchCUJ.pvsched": Metadata{
		BaseTestNames: []string{"ui.TaskSwitchCUJ"},
		Features:      []feature{pvSched},
	},
	"ui.VideoCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.VideoCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.VideoCUJ": Metadata{
		Metrics: videoMetrics,
	},
	"ui.VideoCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.VideoCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.DocsCUJ": Metadata{
		Metrics: defaultMetrics,
	},
	"ui.DocsCUJ.field_trials": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{fieldTrials},
	},
	"ui.DocsCUJ.chromevox": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{chromevox},
	},
	"ui.DocsCUJ.vulkan": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{vulkan},
	},
	"ui.DocsCUJ.image_indexing": Metadata{
		BaseTestNames: []string{"ui.DocsCUJ"},
		Features:      []feature{imageIndexing},
	},
	// The TPS Dashboard assumes that the metrics passed as part of the
	// metadata for SlidesCUJ are the fallback metrics (default metrics) that
	// should be recommended for tests that don't have corresponding metadata.
	// Thus, any changes here would update default metrics for all tests
	// that aren't defined in this map.
	"ui.GoogleSlidesCUJ": Metadata{
		Metrics: defaultMetrics,
	},
}

// WriteMetadataFile stores a metadata.json file in the testing out
// directory representing the test |testName| found in |registry|.
// We log failures if a test is unregistered or there is an issue saving the
// metadata. If a test registration is found, but the registration is
// malformed, the code will panic.
func WriteMetadataFile(ctx context.Context, testName string) {
	if _, ok := Registry[testName]; !ok {
		testing.ContextLogf(ctx, "Failed to find %s in the metadata registry", testName)
		return
	}

	testing.ContextLogf(ctx, "Writing metadata file for %s", testName)

	test := Registry[testName]
	test.TestName = testName

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		testing.ContextLog(ctx, "Failed to get the out directory")
		return
	}

	json, err := json.MarshalIndent(&test, "", "  ")
	if err != nil {
		testing.ContextLog(ctx, "Failed to marshal metadata: ", err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "metadata.json"), json, 0644); err != nil {
		testing.ContextLog(ctx, "Failed to write metadata: ", err)
	}
}
