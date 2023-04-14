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

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// GPUFreqMetrics records the frequency of GPU.
type GPUFreqMetrics struct {
	i915FreqEnabled bool
	i915Freq        perf.Metric
	intervalName    string
}

// Assert that GPUFreqMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &GPUFreqMetrics{}

// GPU actual frequency may be listed as "Actual freq" or "CAGF".
const i915FreqPattern = `(?m)^(?:Actual\sfreq|CAGF): ([0-9]+)`

var i915FreqRe = regexp.MustCompile(i915FreqPattern)

// readI915CurrentFreq reads the frequency of i915 GPU and returns the frequency
// in MHz as an int64.
func readI915CurrentFreq(ctx context.Context) (int64, error) {
	const i915FreqPath = "/sys/kernel/debug/dri/0/i915_frequency_info"
	f, err := os.ReadFile(i915FreqPath)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read i915 frequency info")
	}

	submatchGroup := i915FreqRe.FindStringSubmatch(string(f))
	// A legitimate submatchGroup should look like:
	// ["Actual freq: 200", "200"] or ["CAGF: 500", "500"].
	if len(submatchGroup) < 2 {
		return 0, errors.New("failed to find actual frequency in i915 frequency info file")
	}
	return strconv.ParseInt(strings.TrimSpace(submatchGroup[1]), 10, 64)
}

// NewGPUFreqMetrics creates the struct to store GPU frequency metrics.
func NewGPUFreqMetrics() *GPUFreqMetrics {
	newMetrics := &GPUFreqMetrics{
		i915FreqEnabled: false,
		i915Freq:        perf.Metric{},
		intervalName:    "",
	}
	return newMetrics
}

// Setup creates the metric depending on devices' support on GPU frequency info.
func (g *GPUFreqMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	if _, err := readI915CurrentFreq(ctx); err == nil {
		g.i915FreqEnabled = true
		g.i915Freq = perf.Metric{
			Name:      prefix + gpuFreqMetricType + "gpu_freq",
			Unit:      gpuFreqMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
	}
	return nil
}

// Start logs the start of GPU frequency metrics tracker.
// This function is required by perf.Timeline.
func (g *GPUFreqMetrics) Start(ctx context.Context) error {
	if g.i915FreqEnabled {
		testing.ContextLog(ctx, "Start tracking GPU frequency metrics")
	} else {
		testing.ContextLog(ctx, "Device does not support i915 frequency info")
	}
	return nil
}

// Snapshot logs one snapshot of GPU frequency stat.
func (g *GPUFreqMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if !g.i915FreqEnabled {
		return nil
	}
	v, err := readI915CurrentFreq(ctx)
	if err != nil {
		return err
	}
	values.Append(g.i915Freq, float64(v))
	return nil
}

// Stop logs the stop of GPU frequency metrics tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (g *GPUFreqMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if g.i915FreqEnabled {
		testing.ContextLog(ctx, "Stop tracking GPU frequency metrics")
	}
	return nil
}
