// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/util"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CollectOneTimeMetrics collects necessary additional one-time metrics for
// power dashboard.
func CollectOneTimeMetrics(ctx context.Context) *pb.OneTimeMetrics {
	nonlinear, linear := util.GetBacklightLevel(ctx)

	metrics := new(pb.OneTimeMetrics)
	*metrics = pb.OneTimeMetrics{
		BacklightLinearPercent:    linear,
		BacklightNonlinearPercent: nonlinear,
	}

	if batteryPath, err := pm.SysfsBatteryPath(ctx); err == nil {
		batteryDesignSize, err := pm.ReadBatteryChargeDesignSize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery charge design size: ", err)
		} else {
			metrics.BatteryChargeDesignSize = new(float64)
			*metrics.BatteryChargeDesignSize = batteryDesignSize
		}

		batterySize, err := pm.ReadBatteryChargeSize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery charge size: ", err)
		} else {
			metrics.BatteryChargeSize = new(float64)
			*metrics.BatteryChargeSize = batterySize
		}

		batteryEnergySize, err := pm.ReadBatteryDesignEnergySize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery design energy size: ", err)
		} else {
			metrics.BatteryEnergySize = new(float64)
			*metrics.BatteryEnergySize = batteryEnergySize
		}
	}

	lowBatteryShutdownPercent, err := pm.LowBatteryShutdownPercent(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to read low battery shut down percent: ", err)
	} else {
		metrics.BatteryShutdownPercent = new(float64)
		*metrics.BatteryShutdownPercent = lowBatteryShutdownPercent
	}

	return metrics
}

// GeneratePowerLog generates power_log.{json, html}.
// It will not save perf values and custom perf values
// passed as an OptionalRecorderArg will not be handled.
func GeneratePowerLog(ctx context.Context, outDir, testName string, values *perf.Values, args ...OptionalRecorderArg) error {
	devInfo := GetDeviceInfo(ctx, args...)
	metrics := CollectOneTimeMetrics(ctx)
	pwrLogs, err := cp.CreatePowerLogsAndUpdatePerfValues(ctx, testName, values, nil, devInfo, metrics)
	if err != nil {
		return errors.Wrap(err, "failed to create power log and update perf")
	}
	if err := cp.SavePowerLogs(ctx, outDir, pwrLogs, ""); err != nil {
		return errors.Wrap(err, "failed to save power logs")
	}
	return nil
}

// GeneratePowerLogAndSaveToCrosbolt generates power_log.{json, html},
// and uploads power_log to dashboard.
// It will merge custom perf values passed as OptionalRecorderArg
// and saves perf results to Crosbolt and power_log.{json, html} accordingly.
func GeneratePowerLogAndSaveToCrosbolt(ctx context.Context, outDir, testName string, values *perf.Values, checkpoints *perf.Checkpoints, args ...OptionalRecorderArg) error {
	devInfo := GetDeviceInfo(ctx, args...)
	metrics := CollectOneTimeMetrics(ctx)

	for _, optionalRecorderArg := range args {
		if optionalRecorderArg.argName == OptionalRecorderArgPowerLogCustomPerfKey {
			t := optionalRecorderArg.argValue.(*perf.Values)
			values.Merge(t)
		}
	}

	if _, err := cp.CreateSaveUploadPowerLog(ctx, outDir, testName, "", values, checkpoints, devInfo, metrics); err != nil {
		return errors.Wrap(err, "failed to save and upload power log and perf")
	}

	for _, optionalRecorderArg := range args {
		if optionalRecorderArg.argName == OptionalRecorderArgCustomPerfKey {
			t := optionalRecorderArg.argValue.(*perf.Values)
			values.Merge(t)
		}
	}

	valuesCheckpoints, err := values.FilterTimelineByCheckpoints(checkpoints)
	if err != nil {
		return errors.Wrap(err, "failed to filter data with checkpoints")
	}
	values.Merge(valuesCheckpoints)
	values.SaveAsDebugCSV(outDir)

	if err := values.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data for crosbolt")
	}
	return nil
}
