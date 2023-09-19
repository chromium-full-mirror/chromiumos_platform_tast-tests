// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

// Define a type for each metric. Metric "type" will be added as a prefix before the metric name.
// This is to help categorize each metric in power_log.json/.html, which can also be used as a
// filter on power_dashboard.
const (
	CPUIdleMetricType          = "cpuidle."
	CPUUsageMetricType         = "cpu_usage."
	FanMetricType              = "fan."
	FPSMetricType              = "fps."
	GeneralPerfMetricType      = "perf."
	GPUFreqMetricType          = "gpufreq_wavg."
	GPUUsageMetricType         = "gpu_usage."
	PackageCstatesMetricType   = "cpupkg."
	BatterySOCMetricType       = "battery."
	HistogramMetricType        = "histogram."
	PowerRelatedMetricType     = "power."
	ThermalMetricType          = "temperature."
	WebrtcBitrateMetricType    = "webrtc_bitrate."
	WebrtcFpsMetricType        = "webrtc_fps."
	WebrtcLimitationMetricType = "webrtc_limitation."
	WebrtcPixelMetricType      = "webrtc_pixel."
	WebrtcTimeMetricType       = "webrtc_time."
	WebrtcQPMetricType         = "webrtc_qp."
	ZramMetricType             = "zram."
	MemoryMetricType           = "memory."
)

// Units for each metric type.
const (
	CPUIdleMetricTypeUnit          = "percent"
	CPUUsageMetricTypeUnit         = "percent"
	FanMetricTypeUnit              = "rpm"
	FPSMetricTypeUnit              = "fps"
	GeneralPerfMetricTypeUnit      = "point"
	GPUFreqMetricTypeUnit          = "megahertz"
	GPUUsageUtilizationTypeUnit    = "percent"
	GPUUsageMemoryTypeUnit         = "kiB"
	HistogramLatencyMetricTypeUnit = "us"
	PackageCstatesMetricTypeUnit   = "percent"
	PowerRelatedMetricTypeUnit     = "W"
	ThermalMetricTypeUnit          = "celsius"
	WebrtcBitrateMetricTypeUnit    = "kbps"
	WebrtcFpsMetricTypeUnit        = "fps"
	WebrtcLimitationMetricTypeUnit = "percent"
	WebrtcPixelMetricTypeUnit      = "pixel"
	WebrtcTimeMetricTypeUnit       = "ms"
	WebrtcQPMetricTypeUnit         = "point"
	ZramMetricTypeUnit             = "requests"
	MemoryMetricTypeUnit           = "kiB"
)

// Keys for battery life metrics in power log.
const (
	MinutesBatteryLifeKey       = "minutes_battery_life"
	MinutesBatteryLifeTestedKey = "minutes_battery_life_tested"
)
