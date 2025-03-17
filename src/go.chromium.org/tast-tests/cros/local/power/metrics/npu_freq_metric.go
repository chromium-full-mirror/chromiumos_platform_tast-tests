// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

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

// npuFreqCollector contains all necessary information for reading NPU
// frequency on any given board.
type npuFreqCollector struct {
	// description is a short explanation of what file we are reading from.
	description string

	// path is the location of the file that contains the NPU frequency info.
	path string

	// freqCapturePattern is regexp that can be used to splice the frequency
	// information out of the file at |path|. This pattern should parse out a
	// single group that is the NPU frequency value.
	freqCapturePattern string

	// freqModifier is how much the value read from the frequency file should
	// be divided by to get to MHz. For example, if the file is in Hz,
	// freqModifier would be 1000000.
	freqModifier int
}

// NPUFreqMetrics records the frequency of NPU.
// TODO(giver): Refactor GPUFreqMetrics and NPUFreqMetrics to reuse common codes.
type NPUFreqMetrics struct {
	freqEnabled bool
	freqMetric  perf.Metric
	collector   npuFreqCollector
}

// Assert that NPUFreqMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &NPUFreqMetrics{}

// readNPUFrequency reads the NPU frequency using the npuFreqCollector info.
func readNPUFrequency(ctx context.Context, collector npuFreqCollector) (int64, error) {
	f, err := os.ReadFile(collector.path)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %q", collector.path)
	}

	var freqRe = regexp.MustCompile(collector.freqCapturePattern)
	submatchGroup := freqRe.FindStringSubmatch(string(f))
	if len(submatchGroup) != 2 {
		return 0, errors.Errorf("failed to find NPU frequency info in file: %v", submatchGroup)
	}

	val, err := strconv.ParseInt(strings.TrimSpace(submatchGroup[1]), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse int from substring match")
	}
	return val / int64(collector.freqModifier), nil
}

// NewNPUFreqMetrics creates the struct to store NPU frequency metrics.
func NewNPUFreqMetrics() *NPUFreqMetrics {
	newMetrics := &NPUFreqMetrics{}
	return newMetrics
}

// Setup creates the metric depending on devices' support on NPU frequency info.
func (npu *NPUFreqMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	for _, c := range []npuFreqCollector{{
		description:        "Collect MTK NPU Frequency",
		path:               `/sys/devices/platform/soc/19001000.remoteproc/19001000.remoteproc.mtk_apu_pwr_ipi_tx.6.-1/devfreq/19001000.remoteproc.mtk_apu_pwr_ipi_tx.6.-1/cur_freq`,
		freqCapturePattern: "([0-9]+)",
		freqModifier:       1000000,
	}} {
		if _, err := readNPUFrequency(ctx, c); err == nil {
			testing.ContextLog(ctx, c.description)
			npu.collector = c
			npu.freqEnabled = true
			npu.freqMetric = perf.Metric{
				Name:      prefix + cp.NPUFreqMetricType + "npu_freq",
				Unit:      cp.NPUFreqMetricTypeUnit,
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
				Interval:  intervalName,
			}
			return nil
		}
	}
	return nil
}

// Start logs the start of NPU frequency metrics tracker.
// This function is required by perf.Timeline.
func (npu *NPUFreqMetrics) Start(ctx context.Context) error {
	if npu.freqEnabled {
		testing.ContextLog(ctx, "Start tracking NPU frequency metrics")
	} else {
		testing.ContextLog(ctx, "Device does not support tracking NPU frequency")
	}
	return nil
}

// Snapshot logs one snapshot of NPU frequency stat.
func (npu *NPUFreqMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if !npu.freqEnabled {
		return nil
	}

	v, err := readNPUFrequency(ctx, npu.collector)
	if err != nil {
		return errors.Wrapf(err, "failed to %s", npu.collector.description)
	}
	values.Append(npu.freqMetric, float64(v))
	return nil
}

// Stop logs the stop of NPU frequency metrics tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (npu *NPUFreqMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if npu.freqEnabled {
		testing.ContextLog(ctx, "Stop tracking NPU frequency metrics")
	}
	return nil
}
