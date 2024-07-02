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
}

// GenerateMetadataFile stores a metadata.json file in the testing out
// directory representing |registry|.
func GenerateMetadataFile(ctx context.Context) error {
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
