// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

var timelineSources = []perf.TimelineDatasource{
	NewCpuidleStateMetrics(),
	NewRAPLPowerMetrics(),
	NewSysfsBatteryMetrics(),
	NewSysfsThermalMetrics(),
	NewPackageCStatesMetrics(),
	NewProcfsCPUMetrics(),
	NewFanMetrics(),
	NewGPUStateMetrics(),
	NewGPUFreqMetrics(),
	NewZramIOMetrics(),
}

// TestMetrics returns a slice of metrics that should be used for power tests.
func TestMetrics() []perf.TimelineDatasource {
	// Duplicate the timelineSources into a new slice.
	metricSources := make([]perf.TimelineDatasource, len(timelineSources))
	copy(metricSources, timelineSources)

	return metricSources
}
