// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// getNumFans counts how many fans does the DUT have.
func getNumFans(ctx context.Context) int {
	cmd := "ectool pwmgetnumfans | grep -o [0-9]"
	output, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Cannot get fan numbers from ectool")
		return 0
	}
	num, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		testing.ContextLog(ctx, "Cannot get fan numbers from ectool")
		return 0
	}
	return int(num)
}

// ReadRpm reads the rpm of all fans and returns them as a list.
func ReadRpm(ctx context.Context) ([]int64, error) {
	var rpmData []int64
	output, err := testexec.CommandContext(ctx, "ectool", "pwmgetfanrpm", "all").Output()
	if err != nil {
		return rpmData, errors.Wrap(err, "unable to get RPM from ectool")
	}
	// Trim the last new line to avoid a trailing empty string.
	splitOutput := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, ln := range splitOutput {
		if strings.Contains(ln, "stalled") {
			rpmData = append(rpmData, 0)
			continue
		}
		numString := strings.TrimSpace(strings.Split(ln, ":")[1])
		num, err := strconv.ParseInt(numString, 10, 64)
		if err != nil {
			return rpmData, errors.Wrap(err, "unable to convert read RPM to int64")
		}
		rpmData = append(rpmData, num)
	}
	return rpmData, nil
}

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
	fanNum := getNumFans(ctx)
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
	readData, err := ReadRpm(ctx)
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
