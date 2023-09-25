// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
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

	if batteryPath, err := SysfsBatteryPath(ctx); err == nil {
		batteryDesignSize, err := ReadBatteryChargeDesignSize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery charge design size: ", err)
		} else {
			metrics.BatteryChargeDesignSize = new(float64)
			*metrics.BatteryChargeDesignSize = batteryDesignSize
		}

		batterySize, err := ReadBatteryChargeSize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery charge size: ", err)
		} else {
			metrics.BatteryChargeSize = new(float64)
			*metrics.BatteryChargeSize = batterySize
		}

		batteryEnergySize, err := ReadBatteryDesignEnergySize(ctx, batteryPath)
		if err != nil {
			testing.ContextLog(ctx, "Failed to get battery design energy size: ", err)
		} else {
			metrics.BatteryEnergySize = new(float64)
			*metrics.BatteryEnergySize = batteryEnergySize
		}
	}

	lowBatteryShutdownPercent, err := LowBatteryShutdownPercent(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to read low battery shut down percent: ", err)
	} else {
		metrics.BatteryShutdownPercent = new(float64)
		*metrics.BatteryShutdownPercent = lowBatteryShutdownPercent
	}

	return metrics
}

// GeneratePowerLog returns the power dict and the power log dict, and
// stores power_log.json and power_log.html.
func GeneratePowerLog(ctx context.Context, outDir, testName string, values *perf.Values, args ...OptionalRecorderArg) (map[string]interface{}, map[string]interface{}, error) {
	devInfo := GetDeviceInfo(ctx, args...)
	metrics := CollectOneTimeMetrics(ctx)
	powerDict, err := cp.ConvertPowerPerfValue(ctx, values, metrics)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to convert power perf values to power dictionary")
	}
	powerLogDict := cp.CreatePowerLogDict(ctx, testName, powerDict, devInfo)

	if err := cp.SavePowerLogJSON(ctx, outDir, powerLogDict); err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate power_log.json")
	}

	if err := cp.SavePowerLogHTML(ctx, outDir, powerLogDict); err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate power_log.html")
	}
	return powerDict, powerLogDict, nil
}

// GeneratePowerLogAndSaveToCrosbolt generates power_log.{json, html}
// and upload results to Crosbolt.
func GeneratePowerLogAndSaveToCrosbolt(ctx context.Context, outDir, testName string, values *perf.Values, args ...OptionalRecorderArg) error {
	powerDict, powerLogDict, err := GeneratePowerLog(ctx, outDir, testName, values, args...)
	if err != nil {
		return errors.Wrap(err, "failed to generate power log")
	}

	if powerDict == nil {
		testing.ContextLog(ctx, "Power dictionary is empty. Don't save perf values for crosbolt")
		return nil
	}

	if err := values.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data for crosbolt")
	}

	if err := cp.UploadToDashboard(ctx, powerLogDict, ""); err != nil {
		return errors.Wrap(err, "failed to upload to power dashboard")
	}

	return nil
}
