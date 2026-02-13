// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FanMetrics holds the fan metrics read from procfs.
type FanMetrics struct {
	rpmmetric []perf.Metric
	fanNum    int
}

// Assert that FanMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &FanMetrics{}

// NewFanMetrics creates a struct to capture fan metrics.
func NewFanMetrics() *FanMetrics {
	return &FanMetrics{}
}

// Setup creates the metric.
func (f *FanMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	fanNum := util.GetNumFans(ctx)
	f.fanNum = fanNum
	for i := 0; i < fanNum; i++ {
		newFanMetric := perf.Metric{
			Name:      prefix + cp.FanMetricType + "fan_" + strconv.Itoa(i),
			Unit:      cp.FanMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
		f.rpmmetric = append(f.rpmmetric, newFanMetric)
	}
	return nil
}

// Start logs the start of fan metrics tracker.
// This is required by perf.Timeline even though it practically does nothing.
func (f *FanMetrics) Start(ctx context.Context) error {
	if f.fanNum > 0 {
		testing.ContextLog(ctx, "Start tracking fan RPM")
	} else {
		testing.ContextLog(ctx, "No fan found on DUT")
	}
	return nil
}

// Snapshot takes a snapshot of fan RPM.
func (f *FanMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	if f.fanNum < 1 {
		return nil
	}
	readData, err := util.ReadRpm(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read fan RPM")
	}
	if len(readData) != len(f.rpmmetric) {
		// TODO(b/283697544): Temporarily skip bad fan readings. Find out why
		// fan number changes in the middle of test.
		testing.ContextLogf(ctx, "Fan readings %v has different length from fan metric tracker with length %d",
			readData, len(f.rpmmetric))
		return nil
	}
	for i, singleFanMetric := range f.rpmmetric {
		values.Append(singleFanMetric, float64(readData[i]))
	}
	return nil
}

// Stop logs the stop of fan metrics tracker.
// This is required by perf.Timeline even though it practically does nothing.
func (f *FanMetrics) Stop(ctx context.Context, values *perf.Values) error {
	if f.fanNum > 0 {
		testing.ContextLog(ctx, "Stop tracking fan RPM")
	}
	return nil
}
