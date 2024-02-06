// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metrics contains functions related to power metric measurement.
package metrics

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

// TestMetrics returns a slice of metrics that should be used for power tests.
// Some DUTs have no ChromeEC, do ChromeECSupported check before adding TestMetrics.
func TestMetrics() []perf.TimelineDatasource {
	return append(TestMetricsWithoutBatteryInfo(), NewSysfsBatteryMetrics())
}

// TestMetricsWithoutBatteryInfo returns a slice of metrics that should be used
// for power metrics without battery metrics.
func TestMetricsWithoutBatteryInfo() []perf.TimelineDatasource {
	return []perf.TimelineDatasource{
		NewCpuidleStateMetrics(),
		NewRAPLPowerMetrics(),
		NewSysfsThermalMetrics(),
		NewPackageCStatesMetrics(),
		NewProcfsCPUMetrics(),
		NewFanMetrics(),
		NewGPUUsageDataSource(),
		NewGPUFreqMetrics(),
		NewZramIOMetrics(),
		NewMemoryMetrics(),
	}
}
