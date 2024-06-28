// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metrics contains functions related to power metric measurement.
package metrics

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

// MetricClass is the name of a class of metrics.
type MetricClass string

// List of MetricClass available.
const (
	CPUFreqClass        MetricClass = "cpu_freq"
	CPUIdleStateClass   MetricClass = "cpu_idle_state"
	RAPLPowerClass      MetricClass = "rapl_power"
	SysfsThermalClass   MetricClass = "sysfs_thermal"
	PackageCStatesClass MetricClass = "package_cstates"
	ProcfsCPUClass      MetricClass = "procfs_cpu"
	FanClass            MetricClass = "fan"
	GPUUsageClass       MetricClass = "gpu_usage"
	GPUFreqClass        MetricClass = "gpu_freq"
	ZramIOClass         MetricClass = "zram_io"
	MemoryClass         MetricClass = "memory"
	SysfsBatteryClass   MetricClass = "sysfs_battery"
)

// TestMetrics returns a slice of metrics that should be used for power tests.
// Some DUTs have no ChromeEC, do ChromeECSupported check before adding TestMetrics.
func TestMetrics(classes ...MetricClass) []perf.TimelineDatasource {
	if len(classes) == 0 {
		return append(TestMetricsWithoutBatteryInfo(), NewSysfsBatteryMetrics())
	}

	var metrics []perf.TimelineDatasource
	for _, class := range classes {
		switch class {
		case CPUFreqClass:
			metrics = append(metrics, NewCPUUsageSource("cpu_freq", false))
		case CPUIdleStateClass:
			metrics = append(metrics, NewCpuidleStateMetrics())
		case RAPLPowerClass:
			metrics = append(metrics, NewRAPLPowerMetrics())
		case SysfsThermalClass:
			metrics = append(metrics, NewSysfsThermalMetrics())
		case PackageCStatesClass:
			metrics = append(metrics, NewPackageCStatesMetrics())
		case ProcfsCPUClass:
			metrics = append(metrics, NewProcfsCPUMetrics())
		case FanClass:
			metrics = append(metrics, NewFanMetrics())
		case GPUUsageClass:
			metrics = append(metrics, NewGPUUsageDataSource())
		case GPUFreqClass:
			metrics = append(metrics, NewGPUFreqMetrics())
		case ZramIOClass:
			metrics = append(metrics, NewZramIOMetrics())
		case MemoryClass:
			metrics = append(metrics, NewMemoryMetrics())
		case SysfsBatteryClass:
			metrics = append(metrics, NewSysfsBatteryMetrics())
		}
	}
	return metrics

}

// TestMetricsWithoutBatteryInfo returns a slice of metrics that should be used
// for power metrics without battery metrics.
func TestMetricsWithoutBatteryInfo() []perf.TimelineDatasource {
	return []perf.TimelineDatasource{
		NewCPUUsageSource("cpu_freq", false),
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
