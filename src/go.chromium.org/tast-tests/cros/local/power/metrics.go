// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

// TestMetrics returns a slice of metrics that should be used for power tests.
func TestMetrics() []perf.TimelineDatasource {
	return []perf.TimelineDatasource{
		NewCpuidleStateMetrics(),
		NewRAPLPowerMetrics(),
		NewSysfsBatteryMetrics(),
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
