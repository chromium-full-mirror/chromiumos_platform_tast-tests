// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// HistogramMetrics records the histogram stat.
type HistogramMetrics struct {
	tconn     *chrome.TestConn
	names     []string
	metrics   map[string]perf.Metric
	lastHists []*metrics.Histogram
}

// Assert that HistogramMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &HistogramMetrics{}

type histogramMetadata struct {
	unit      string
	direction perf.Direction
}

// A map of supported histogram name to {unit, direction}
var validHistogramsMap = map[string]histogramMetadata{
	"Event.Latency.EndToEnd.KeyPress":      {cp.HistogramLatencyMetricTypeUnit, perf.SmallerIsBetter},
	"EventLatency.KeyPressed.TotalLatency": {cp.HistogramLatencyMetricTypeUnit, perf.SmallerIsBetter},
}

// NewHistogramMetrics creates the struct to store Chrome histogram metrics.
func NewHistogramMetrics(tconn *chrome.TestConn, names []string) *HistogramMetrics {
	newMetrics := &HistogramMetrics{
		tconn:   tconn,
		names:   names,
		metrics: make(map[string]perf.Metric),
	}
	return newMetrics
}

// Setup creates metrics.
func (v *HistogramMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	metrics.ClearHistogramTransferFile()
	for _, name := range v.names {
		if _, ok := validHistogramsMap[name]; !ok {
			return errors.Errorf("unexpected histogram name %s", name)
		}
		// Remove "." from the name because power_dashboard would confuse that with metric type.
		nameInChart := strings.ReplaceAll(name, ".", "")
		v.metrics[name] = perf.Metric{
			Name:      prefix + cp.HistogramMetricType + nameInChart,
			Unit:      validHistogramsMap[name].unit,
			Direction: validHistogramsMap[name].direction,
			Multiple:  true,
			Interval:  intervalName,
		}
	}
	return nil
}

// Start logs the start of histogram stats tracker.
func (v *HistogramMetrics) Start(ctx context.Context) error {
	testing.ContextLog(ctx, "Start tracking histogram stats")
	histograms, err := metrics.GetHistograms(ctx, v.tconn, v.names)
	if err != nil {
		return errors.Wrapf(err, "failed to get histograms %v", v.names)
	}
	v.lastHists = histograms
	return nil
}

// Snapshot takes one snapshot of histogram stats.
func (v *HistogramMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	newHists, err := metrics.GetHistograms(ctx, v.tconn, v.names)
	if err != nil {
		return errors.Wrapf(err, "failed to get histograms %v", v.names)
	}
	diffs, err := metrics.DiffHistograms(v.lastHists, newHists)
	if err != nil {
		return errors.Wrapf(err, "failed to diff histograms, old had length %d; new had length %d", len(v.lastHists), len(newHists))
	}
	for _, h := range diffs {
		mean, err := h.Mean()
		if err != nil {
			return errors.Wrapf(err, "failed to get mean for histogram %s", h.Name)
		}
		values.Append(v.metrics[h.Name], mean)
	}
	v.lastHists = newHists
	return nil
}

// Stop logs the stop of histogram stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (v *HistogramMetrics) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLog(ctx, "Stop tracking histogram stats")
	return nil
}
