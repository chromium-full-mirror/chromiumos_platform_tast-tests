// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ServodMetrics records servod readings from the DUT.
type ServodMetrics struct {
	svo        *servo.Servo
	rails      []servo.FloatControl
	clearRails []servo.IntControl
	cpd        bool
	pv         *perf.Values
	metrics    map[string]perf.Metric
}

const (
	// CpdPrefix is a special prefix on CPD rails.
	CpdPrefix        = "ft4232h_generic"
	servoAccumSuffix = "_mw"
)

// Assert that ServodMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &ServodMetrics{}

// NewServodMetrics creates a timeline metric to store servod readings.
func NewServodMetrics(ctx context.Context, svo *servo.Servo, cpd bool, filters ...*regexp.Regexp) (*ServodMetrics, error) {
	if cpd {
		filters = append(filters, regexp.MustCompile(CpdPrefix))
	}
	// Query for available accumulator rails.
	rails, clearRails, err := servo.FindAccumRailsWithFilter(ctx, svo, filters)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get accum rails")
	}
	testing.ContextLog(ctx, "Avg power rail commands found:", rails)

	return &ServodMetrics{
		svo:        svo,
		rails:      rails,
		clearRails: clearRails,
		cpd:        cpd,
		pv:         perf.NewValues(),
		metrics:    make(map[string]perf.Metric),
	}, nil
}

// Setup initialises a metric for each rail.
func (m *ServodMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	for _, rail := range m.rails {
		name := m.trimRailName(string(rail))
		m.metrics[string(rail)] = perf.Metric{
			Name:      prefix + name,
			Unit:      ServodMetricTypeUnit,
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
			Interval:  intervalName,
		}
	}
	return nil
}

// Start clears the servod accumulators.
func (m *ServodMetrics) Start(ctx context.Context) error {
	if err := servo.ClearServoAccumulators(ctx, m.svo, m.clearRails); err != nil {
		return errors.Wrap(err, "unable to clear servo accumulators")
	}
	return nil
}

// Snapshot queries accumulator rails and current timestamp, and clears accumulators.
// Note: The function may take longer depending on the number of rails queried.
func (m *ServodMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	for _, rail := range m.rails {
		mw, err := m.svo.GetFloat(ctx, rail)
		if err != nil {
			return errors.Wrapf(err, "failed to get mw from servo instance: %s", string(rail))
		}
		values.Append(m.metrics[string(rail)], mw)
	}

	if err := servo.ClearServoAccumulators(ctx, m.svo, m.clearRails); err != nil {
		return errors.Wrap(err, "unable to clear servo accumulators")
	}

	return nil
}

// Stop does nothing.
func (m *ServodMetrics) Stop(_ context.Context, _ *perf.Values) error {
	return nil
}

func (m *ServodMetrics) trimRailName(name string) string {
	if m.cpd {
		name = name[len(CpdPrefix)+1 : len(name)]
	}
	return name[:len(name)-len(servoAccumSuffix)]
}
