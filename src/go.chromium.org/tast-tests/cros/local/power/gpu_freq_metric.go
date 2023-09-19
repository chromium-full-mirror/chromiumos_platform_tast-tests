// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// gpuFreqCollector contains all necessary information for reading GPU
// frequency on any given board.
type gpuFreqCollector struct {
	// description is a short explanation of what file we are reading from.
	description string

	// path is the location of the file that contains the GPU frequency info.
	path string

	// freqCapturePattern is regexp that can be used to splice the frequency
	// information out of the file at |path|. This pattern should parse out a
	// single group that is the GPU frequency value.
	freqCapturePattern string

	// freqModifier is how much the value read from the frequency file should
	// be divided by to get to MHz. For example, if the file is in Hz,
	// freqModifier would be 1000000.
	freqModifier int
}

// GPUFreqMetrics records the frequency of GPU.
type GPUFreqMetrics struct {
	freqEnabled bool
	freqMetric  perf.Metric
	collector   gpuFreqCollector
}

// Assert that GPUFreqMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &GPUFreqMetrics{}

// readGPUFrequency reads the GPU frequency using the gpuFreqCollector info.
func readGPUFrequency(ctx context.Context, collector gpuFreqCollector) (int64, error) {
	f, err := os.ReadFile(collector.path)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %q", collector.path)
	}

	var freqRe = regexp.MustCompile(collector.freqCapturePattern)
	submatchGroup := freqRe.FindStringSubmatch(string(f))
	if len(submatchGroup) != 2 {
		return 0, errors.Errorf("failed to find GPU frequency info in file: %v", submatchGroup)
	}

	val, err := strconv.ParseInt(strings.TrimSpace(submatchGroup[1]), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse int from substring match")
	}
	return val / int64(collector.freqModifier), nil
}

// NewGPUFreqMetrics creates the struct to store GPU frequency metrics.
func NewGPUFreqMetrics() *GPUFreqMetrics {
	newMetrics := &GPUFreqMetrics{}
	return newMetrics
}

// Setup creates the metric depending on devices' support on GPU frequency info.
func (g *GPUFreqMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	for _, c := range []gpuFreqCollector{{
		description: "Collect i915 GPU Frequency",
		path:        "/sys/kernel/debug/dri/0/i915_frequency_info",
		// GPU actual frequency may be listed as "Actual freq" or "CAGF".
		freqCapturePattern: `(?m)^(?:Actual\sfreq|CAGF): ([0-9]+)`,
		freqModifier:       1,
	}, {
		description:        "Collect AMD GPU Frequency",
		path:               "/sys/kernel/debug/dri/0/amdgpu_pm_info",
		freqCapturePattern: `([0-9]*)\sMHz\s\(SCLK\)`,
		freqModifier:       1,
	}, {
		description:        "Collect QC GPU Frequency",
		path:               `/sys/devices/platform/soc@0/5000000.gpu/devfreq/5000000.gpu/cur_freq`,
		freqCapturePattern: "([0-9]+)",
		freqModifier:       1000000,
	}} {
		if _, err := readGPUFrequency(ctx, c); err == nil {
			testing.ContextLog(ctx, c.description)
			g.collector = c
			g.freqEnabled = true
			g.freqMetric = perf.Metric{
				Name:      prefix + cp.GPUFreqMetricType + "gpu_freq",
				Unit:      cp.GPUFreqMetricTypeUnit,
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  intervalName,
			}
			return nil
		}
	}
	return nil
}

// Start logs the start of GPU frequency metrics tracker.
// This function is required by perf.Timeline.
func (g *GPUFreqMetrics) Start(ctx context.Context) error {
	if g.freqEnabled {
		testing.ContextLog(ctx, "Start tracking GPU frequency metrics")
	} else {
		testing.ContextLog(ctx, "Device does not support tracking GPU frequency")
	}
	return nil
}

// Snapshot logs one snapshot of GPU frequency stat.
func (g *GPUFreqMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if !g.freqEnabled {
		return nil
	}

	v, err := readGPUFrequency(ctx, g.collector)
	if err != nil {
		return errors.Wrapf(err, "failed to %s", g.collector.description)
	}
	values.Append(g.freqMetric, float64(v))
	return nil
}

// Stop logs the stop of GPU frequency metrics tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (g *GPUFreqMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if g.freqEnabled {
		testing.ContextLog(ctx, "Stop tracking GPU frequency metrics")
	}
	return nil
}
