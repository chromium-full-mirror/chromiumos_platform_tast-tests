// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strings"

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
	pv         *perf.Values
	metrics    map[string]perf.Metric
}

// Assert that ServodMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &ServodMetrics{}

// NewServodMetrics creates a timeline metric to store servod readings.
func NewServodMetrics(ctx context.Context, svo *servo.Servo, useAccumulators bool, filters ...*regexp.Regexp) (*ServodMetrics, error) {
	// Query for available rails.
	rails, clearRails, err := servo.FindPowerRailsWithFilter(ctx, svo, useAccumulators, filters)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find servo power rails")
	}

	if useAccumulators {
		rails, clearRails, err = filterInvalidAccumulatorRails(ctx, svo, rails, clearRails)
		if err != nil {
			return nil, errors.Wrap(err, "failed to filter invalid rails")
		}
	}

	testing.ContextLog(ctx, "Power rail commands found: ", rails)

	return &ServodMetrics{
		svo:        svo,
		rails:      rails,
		clearRails: clearRails,
		pv:         perf.NewValues(),
		metrics:    make(map[string]perf.Metric),
	}, nil
}

// Setup initialises a metric for each rail.
func (m *ServodMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	for _, rail := range m.rails {
		name := trimRailName(string(rail))
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

func trimRailName(name string) string {
	// Remove 'ft4232h_generic.' prefix and '_mw' suffix
	re := regexp.MustCompile(`^(?:ft4232h_generic\.)?([[:alnum:]-_]+)_mw$`)
	m := re.FindStringSubmatch(name)
	if m == nil {
		return name
	}
	return m[1]
}

func filterInvalidAccumulatorRails(ctx context.Context, svo *servo.Servo, rails []servo.FloatControl, clearRails []servo.IntControl) ([]servo.FloatControl, []servo.IntControl, error) {

	// Split rails into CPD and non-CPD.
	var cpdRails []servo.FloatControl
	var nonCpdRails []servo.FloatControl

	for _, r := range rails {
		if strings.HasPrefix(string(r), "ft4232h_generic") {
			cpdRails = append(cpdRails, r)
		} else {
			nonCpdRails = append(nonCpdRails, r)
		}
	}

	var filteredRails []servo.FloatControl
	var filteredClearRails []servo.IntControl
	accumRegexp := regexp.MustCompile("_avg_mw$")

	// Check if each CPD rail is working.
	for _, r := range cpdRails {
		if _, err := svo.GetFloat(ctx, r); err == nil {
			filteredRails = append(filteredRails, r)
			clearRail := accumRegexp.ReplaceAllString(string(r), "_acc_clear")
			filteredClearRails = append(filteredClearRails, servo.IntControl(clearRail))
		}
	}

	// CPD always have 2 working rails (vbat and vbat_alt).
	// If there is any more rails from CPD works, use it.
	// Otherwise, check non-CPD rails.
	var invalidRails []string
	if len(filteredRails) <= 2 {
		for _, r := range nonCpdRails {
			if _, err := svo.GetFloat(ctx, r); err == nil {
				filteredRails = append(filteredRails, r)
				clearRail := accumRegexp.ReplaceAllString(string(r), "_acc_clear")
				filteredClearRails = append(filteredClearRails, servo.IntControl(clearRail))
			} else {
				invalidRails = append(invalidRails, string(r))
			}
		}
	}

	// If the rail is still filtered without CPD, it may mean that the conf overlay is incorrect. Print them to help debugging.
	testing.ContextLog(ctx, "Filtered out invalid accum rails: ", invalidRails)

	return filteredRails, filteredClearRails, nil
}
