// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// power log file name
const powerLogFileName = "power_log"

// ConvertPowerPerfValue converts raw performance metric values to power dictionary.
func ConvertPowerPerfValue(ctx context.Context, values *perf.Values) map[string]interface{} {
	measurement := values.GetValues()
	if measurement == nil || len(measurement) == 0 {
		testing.ContextLog(ctx, "No valid power measurement")
		return nil
	}

	powerDict := map[string]interface{}{
		// sample_count indicates how many time each metric has been collected
		"sample_count": 0,
		// sample_duration is the time interval between two data points
		"sample_duration": 0,
		// average is the mean of each metric
		"average": nil,
		// data is all the data points collected for each metric
		"data": nil,
	}

	innerDataMap := make(map[string][]float64)
	innerAverageMap := make(map[string]float64)

	for metric, value := range measurement {
		innerDataMap[metric.Name] = value
		sum := 0.0
		for _, num := range value {
			sum += num
		}

		var mean float64
		if len(value) != 0 {
			mean = sum / float64(len(value))
		}

		innerAverageMap[metric.Name] = mean
	}

	powerDict["data"] = innerDataMap
	powerDict["average"] = innerAverageMap

	if value, ok := innerDataMap["t"]; ok {
		var sampleCount = len(value)
		powerDict["sample_count"] = sampleCount
		if sampleCount > 1 {
			var totalDuration = value[sampleCount-1] - value[0]
			powerDict["sample_duration"] = totalDuration / (float64(sampleCount) - 1)
		}
	}

	return powerDict
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

// GeneratePowerLogAndSaveToCrosbolt generates power_log.json and upload results to crosbolt.
func GeneratePowerLogAndSaveToCrosbolt(ctx context.Context, outDir, testName string, values *perf.Values) error {
	powerDict := ConvertPowerPerfValue(ctx, values)

	powerLogDict := CreatePowerLogDict(ctx, testName, powerDict)

	if err := SavePowerLogJSON(ctx, outDir, powerLogDict); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json")
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
