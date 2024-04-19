// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import "time"

const (
	// RecorderTempCooldownTimeout is the max amount of time allowed for device
	// temperature to cooldown.
	RecorderTempCooldownTimeout = 5 * time.Minute
	// RecorderIdleStateCooldownTimeout is the max amount of time allowed for cpu
	// idle state activity to drop.
	RecorderIdleStateCooldownTimeout = 2 * time.Minute
	// RecorderPkgStateCooldownTimeout is the max amount of time allowed for cpu
	// pkg state activity to drop.
	RecorderPkgStateCooldownTimeout = 3 * time.Minute

	// RecorderCooldownTimeout is the max amount of time that Recorder allows
	// for all combined cooldown items.
	RecorderCooldownTimeout = RecorderTempCooldownTimeout + RecorderIdleStateCooldownTimeout +
		RecorderPkgStateCooldownTimeout

	// RecorderOverheadTimeout is the max amount of time needed for recorder &
	// metrics construction, recorder destruction and data post-processing.
	RecorderOverheadTimeout = DashboardUploadTimeout + 8*time.Minute

	// RecorderTimeout is the max amount of time that Recorder is expected to
	// take.
	RecorderTimeout = RecorderCooldownTimeout + RecorderOverheadTimeout

	// OptionalRecorderArgCustomPerfKey is the key used to get optional custom
	// perf values saved only to results-chart.json.
	// Be very careful when you add optional custom perf values with a timeline,
	// it may overwrite the timeline in power recorder metrics.
	OptionalRecorderArgCustomPerfKey = "custom_perf_results_chart"

	// OptionalRecorderArgPowerLogCustomPerfKey is the key used to get optional
	// custom perf values saved to both power logs and results-chart.json.
	// Be very careful when you add custom perf values with a timeline,
	// it may overwrite the timeline in power recorder metrics.
	OptionalRecorderArgPowerLogCustomPerfKey = "custom_perf_power_log"

	// OptionalRecorderArgDischargeWatchdogKey is the arg name to enable
	// discharge watchdog in recorder.
	OptionalRecorderArgDischargeWatchdogKey = "enable_watchdog"
)
