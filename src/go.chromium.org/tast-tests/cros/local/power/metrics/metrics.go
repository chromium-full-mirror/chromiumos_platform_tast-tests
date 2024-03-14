// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metrics contains functions related to power metric measurement.
package metrics

import (
	"go.chromium.org/tast-tests/cros/common/perf"
)

// MetricClassName is the name of a class of metrics.
type MetricClassName string

// MetricName describes one metric in a metric class.
type MetricName string

// MetricClass is a type for user to specify metric class+name subsets.
type MetricClass struct {
	Class MetricClassName
	// TODO: b/293221069 - add per-class metric filtering.
	// names []MetricName
}

// List of MetricClass available.
const (
	CPUIdleStateClass   MetricClassName = "cpu_idle_state"
	RAPLPowerClass      MetricClassName = "rapl_power"
	SysfsThermalClass   MetricClassName = "sysfs_thermal"
	PackageCStatesClass MetricClassName = "package_cstates"
	ProcfsCPUClass      MetricClassName = "procfs_cpu"
	FanClass            MetricClassName = "fan"
	GPUUsageClass       MetricClassName = "gpu_usage"
	GPUFreqClass        MetricClassName = "gpu_freq"
	ZramIOClass         MetricClassName = "zram_io"
	MemoryClass         MetricClassName = "memory"
	SysfsBatteryClass   MetricClassName = "sysfs_battery"
)

// TestMetrics returns a slice of metrics that should be used for power tests.
// Some DUTs have no ChromeEC, do ChromeECSupported check before adding TestMetrics.
func TestMetrics(classes ...MetricClass) []perf.TimelineDatasource {
	if len(classes) == 0 {
		return append(TestMetricsWithoutBatteryInfo(), NewSysfsBatteryMetrics())
	}

	var metrics []perf.TimelineDatasource
	for _, class := range classes {
		switch class.Class {
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
