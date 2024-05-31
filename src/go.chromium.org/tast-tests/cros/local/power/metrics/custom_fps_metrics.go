// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"

	"go.chromium.org/tast/core/testing"
)

// CustomFpsMetrics records the fps with given function.
type CustomFpsMetrics struct {
	getFPSFunc   func() float64
	name         string
	metrics      map[string]perf.Metric
	prefix       string
	intervalName string
}

// Assert that CustomFpsMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &CustomFpsMetrics{}

// NewCustomFpsMetrics creates the struct to store custom FPS metrics.
func NewCustomFpsMetrics(getFPSFunc func() float64, name string) *CustomFpsMetrics {
	newMetrics := &CustomFpsMetrics{
		getFPSFunc: getFPSFunc,
		name:       name,
		metrics:    make(map[string]perf.Metric),
	}
	return newMetrics
}

// Setup collects the prefix and interval name.
func (v *CustomFpsMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	v.prefix = prefix
	v.intervalName = intervalName
	return nil
}

// Start logs the start of fps stats tracker.
// This function is required by perf.Timeline.
func (v *CustomFpsMetrics) Start(ctx context.Context) error {
	v.metrics[v.name] = perf.Metric{
		Name:      v.prefix + cp.FPSMetricType + v.name,
		Unit:      cp.FPSMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  v.intervalName,
	}
	return nil
}

// Snapshot takes one snapshot of fps stats.
func (v *CustomFpsMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	values.Append(v.metrics["fps"], v.getFPSFunc())
	return nil
}

// Stop logs the stop of fps stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (v *CustomFpsMetrics) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLog(ctx, "Stop tracking custom fps stats")
	return nil
}
