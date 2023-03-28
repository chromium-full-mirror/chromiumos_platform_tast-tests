// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// Define a type for each metric. Metric "type" will be added as a prefix before the metric name.
// This is to help categorize each metric in power_log.json/.html, which can also be used as a
// filter on power_dashboard.
const (
	cpuIdleMetricType        = "cpuidle."
	cpuUsageMetricType       = "cpu_usage."
	fanMetricType            = "fan."
	generalPerfMetricType    = "perf."
	gpuFreqMetricType        = "gpufreq_wavg."
	gpuStateMetricType       = "gpuidle."
	packageCstatesMetricType = "cpupkg."
	powerRelatedMetricType   = "power."
	thermalMetricType        = "temperature."
	zramMetricType           = "zram."
)

// Only keys inside validMetricTypeMap are accepted metric types.
var validMetricTypeMap = map[string]bool{
	"cpuidle":      true,
	"cpu_usage":    true,
	"fan":          true,
	"perf":         true,
	"gpufreq_wavg": true,
	"gpuidle":      true,
	"cpupkg":       true,
	"power":        true,
	"temperature":  true,
	"zram":         true,
}

// Units for each metric type.
const (
	cpuIdleMetricTypeUnit        = "percent"
	cpuUsageMetricTypeUnit       = "ratio"
	fanMetricTypeUnit            = "rpm"
	gpuFreqMetricTypeUnit        = "MHz"
	gpuStateMetricTypeUnit       = "percent"
	packageCstatesMetricTypeUnit = "percent"
	powerRelatedMetricTypeUnit   = "W"
	thermalMetricTypeUnit        = "deg_C"
	zramMetricTypeUnit           = "requests"
)

// Power log file name.
const powerLogFileName = "power_log"

// ConvertPowerPerfValue converts raw performance metric values to power dictionary.
func ConvertPowerPerfValue(ctx context.Context, values *perf.Values) (map[string]interface{}, error) {
	measurement := values.GetValues()
	if measurement == nil || len(measurement) == 0 {
		return nil, errors.New("invalid power measurement")
	}

	powerDict := map[string]interface{}{
		// sample_count indicates how many time each metric has been collected.
		"sample_count": 0,
		// sample_duration is the time interval between two data points.
		"sample_duration": 0,
		// average is the mean of each metric.
		"average": nil,
		// data is all the data points collected for each metric.
		"data": nil,
		// type is a map from metric to type.
		"metric_type": nil,
		// unit is a map from metric to unit.
		"metric_unit": nil,
	}

	innerDataMap := make(map[string][]float64)
	innerAverageMap := make(map[string]float64)
	typeMap := make(map[string]string)
	unitMap := make(map[string]string)

	for metric, value := range measurement {
		// metric.Name is a string of "(prefix.)(metricType.)metricName"
		metricNameSlice := strings.Split(metric.Name, ".")
		var metricType string
		var metricName string

		size := len(metricNameSlice)

		// metric.Name is guaranteed be non-empty string.
		metricName = metricNameSlice[size-1]

		// Four scenarios to be considered:
		// 1."system": used both by power and ARCVM team. To minimize interuption,
		// a power metric type is not assigned to it. Manually adding a metric type
		// for "system" because metric.Name doesn't include a metric type.
		// 2. "t": not a power metric, therefore a metric type isn't assigned and
		// it will not be included in the typeMap.
		// 3. size == 1: indicating no metricType assigned, throw an error.
		// 4. Retrieve metricType from metricNameSlice.
		if metricName == "system" {
			metricType = "power"
		} else if metricName == "t" {
			innerDataMap[metricName] = value
			continue
		} else if size == 1 {
			return nil, errors.Errorf("failed to parse metric %q, could not find the metric type", metricName)
		} else {
			metricType = metricNameSlice[size-2]
		}

		// Validate metricType.
		if _, ok := validMetricTypeMap[metricType]; !ok {
			return nil, errors.Errorf("unexpected metric type %q for %q", metricType, metricName)
		}

		typeMap[metricName] = metricType
		unitMap[metricName] = metric.Unit
		innerDataMap[metricName] = value

		sum := 0.0
		for _, num := range value {
			sum += num
		}

		var mean float64
		if len(value) != 0 {
			mean = sum / float64(len(value))
		}

		innerAverageMap[metricName] = mean
	}

	var totalDurationSec float64
	if value, ok := innerDataMap["t"]; ok {
		var sampleCount = len(value)
		powerDict["sample_count"] = sampleCount
		if sampleCount > 1 {
			totalDurationSec = value[sampleCount-1] - value[0]
			powerDict["sample_duration"] = totalDurationSec / (float64(sampleCount) - 1)
		}
	}

	MinutesBatteryLife := getMinutesBatteryLife(ctx, innerDataMap, innerAverageMap, totalDurationSec)
	values.Set(perf.Metric{
		Name:      "minutes_battery_life",
		Unit:      "minute",
		Direction: perf.BiggerIsBetter,
	}, MinutesBatteryLife)
	innerDataMap["minutes_battery_life"] = []float64{MinutesBatteryLife}
	innerAverageMap["minutes_battery_life"] = MinutesBatteryLife

	values.Set(perf.Metric{
		Name:      "minutes_battery_life_tested",
		Unit:      "minute",
		Direction: perf.BiggerIsBetter,
	}, totalDurationSec/60.0)
	innerDataMap["minutes_battery_life_tested"] = []float64{totalDurationSec / 60.0}
	innerAverageMap["minutes_battery_life_tested"] = totalDurationSec / 60.0

	powerDict["data"] = innerDataMap
	powerDict["average"] = innerAverageMap
	powerDict["metric_type"] = typeMap
	powerDict["metric_unit"] = unitMap
	return powerDict, nil
}

// getMinutesBatteryLife calculates and returns the projected operating minutes.
func getMinutesBatteryLife(ctx context.Context,
	innerDataMap map[string][]float64,
	innerAverageMap map[string]float64,
	totalDurationSec float64) float64 {
	// Power key value calculation.
	var MinutesBatteryLife float64
	var energyFull float64
	batteryPath, err := SysfsBatteryPath(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to calculate key value: ", err)
		return MinutesBatteryLife
	}

	energyFull, err = ReadBatterySize(ctx, batteryPath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get battery size: ", err)
		return MinutesBatteryLife
	}

	if energyUsed, ok := innerAverageMap["discharge_mwh"]; ok && energyUsed > 0 && totalDurationSec > 0 {
		lowBatteryShutdownPercent, err := LowBatteryShutdownPercent(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Failed to read low battery shut down percent: Use 4% for approximation")
			lowBatteryShutdownPercent = 4.0
		}
		batSizeScale := 1 - lowBatteryShutdownPercent/100.0

		var chargeUsedInPercent float64
		chargeValue, exist := innerDataMap["battery_soc"]
		if exist && len(chargeValue) > 1 {
			chargeUsedInPercent = chargeValue[len(chargeValue)-1] - chargeValue[0]
		}
		// For longer tests (> 1hr), charge (Ah) consumption is more accurate for calculating projected battery life.
		// For shorter tests (< 1hr), energy (Wh) consumption is more accurate for calculating projected battery life.
		const MinReasonableDuration = 3600
		if totalDurationSec > MinReasonableDuration && chargeUsedInPercent > 0 {
			// Use charge to project operation time when test run time > 1 hour.
			chargeRate := chargeUsedInPercent / (totalDurationSec / 60.0)
			MinutesBatteryLife = batSizeScale / chargeRate
		} else {
			// Use energy to project operation time when test run time < 1 hour.
			// Notice energyUsed is in mWh and battery size is in Wh. energyRate is in Wh/min.
			energyRate := energyUsed / (totalDurationSec / 60.0) / 1000.0
			MinutesBatteryLife = energyFull * batSizeScale / energyRate
		}
	} else {
		// If energy used is 0 (test too short to cover valid samplings, test did not run on battery, ...):
		// Log that we will not calculate minutes_battery_life.
		testing.ContextLog(ctx, "Failed to calculate minutes_battery_life: 0 energy usage")
	}
	return MinutesBatteryLife
}

// CreatePowerLogDict creates the power log dictionary from power dict.
func CreatePowerLogDict(ctx context.Context, testName string, powerDict map[string]interface{}) map[string]interface{} {
	powerLogDict := map[string]interface{}{
		"format_version": 7,
		// TODO: see b/271917877
		// This is the start time of the test
		// Unfortunately, not every tracker records time
		// We can probably ignore this for now
		// We also need to add the following entry to
		// 'google3/experimental/chromeos_power/dashboard/bigquery_schema.json':
		// {
		// 	"description": "Unix timestamp when the test start running.",
		// 	"mode": "NULLABLE",
		// 	"name": "timestamp",
		// 	"type": "TIMESTAMP"
		// 	},
		"timestamp": time.Now().Unix(),
		"test":      testName,
		"dut":       GetDeviceInfo(ctx),
		"power":     powerDict,
	}

	return powerLogDict
}

// SavePowerLogJSON saves the power log as a json file format.
func SavePowerLogJSON(ctx context.Context, outDir string, powerLogDict map[string]interface{}) error {
	filePath := path.Join(outDir, powerLogFileName+".json")
	j, err := json.MarshalIndent(powerLogDict, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshall data for %s json file", powerLogFileName)
	}
	if err := ioutil.WriteFile(filePath, j, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s json file", powerLogFileName)
	}

	return nil
}

// UploadToDashboard uploads the power test metrics to go/power-dashboard-view.
func UploadToDashboard(ctx context.Context, powerLogDict map[string]interface{}, uploadurl string) error {
	var urlActual string
	if uploadurl == "" {
		urlActual = "http://chrome-power.appspot.com/rapl"
	} else {
		urlActual = uploadurl
	}
	powerLogJSON, err := json.Marshal(powerLogDict)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data when uploading to dashboard")
	}
	urlParams := url.Values{}
	urlParams.Add("data", string(powerLogJSON))
	if _, err = http.PostForm(urlActual, urlParams); err != nil {
		return errors.Wrap(err, "failed to upload to power dashboard")
	}
	return nil
}

// GeneratePowerLogAndSaveToCrosbolt generates power_log.json and upload results to crosbolt.
func GeneratePowerLogAndSaveToCrosbolt(ctx context.Context, outDir, testName string, values *perf.Values) error {
	powerDict, err := ConvertPowerPerfValue(ctx, values)
	if err != nil {
		return errors.Wrap(err, "failed to convert power perf values to power dictionary")
	}
	powerLogDict := CreatePowerLogDict(ctx, testName, powerDict)

	if err := SavePowerLogJSON(ctx, outDir, powerLogDict); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json")
	}

	if err := UploadToDashboard(ctx, powerLogDict, ""); err != nil {
		return errors.Wrap(err, "failed to upload to power dashboard")
	}

	if powerDict == nil {
		testing.ContextLog(ctx, "Power dictionary is empty. Don't save perf values for crosbolt")
		return nil
	}

	if err := values.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data for crosbolt")
	}

	return nil
}
