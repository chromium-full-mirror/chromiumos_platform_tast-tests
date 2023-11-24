// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package remotepower includes helper functions for remote power tests.
package remotepower

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
)

const (
	// IntervalMetricName is the name of the key for perf interval metric.
	IntervalMetricName = "t"
	// ServoIntervalMetricName is a custom name for the key for servo interval metric.
	ServoIntervalMetricName = "servod.t"
)

// SubtestResult is the mapping to the results.json object.
type SubtestResult struct {
	Name  string `json:"name"`
	Start string `json:"start"`
}

// findSubtestStartTime gets the start timestamp (seconds since January 1, 1970)
// of a test from the perf results.json file
func findSubtestStartTime(subtestDir string) (float64, error) {
	var results []SubtestResult
	rf, err := os.Open(filepath.Join(subtestDir, "results.json"))
	if err != nil {
		return 0.0, errors.Wrap(err, "couldn't open results file")
	}
	defer rf.Close()

	if err = json.NewDecoder(rf).Decode(&results); err != nil {
		return 0.0, errors.Wrapf(err, "couldn't decode results from %v", rf.Name())
	}
	if len(results) > 1 {
		return 0.0, errors.New("more than one subtest was run")
	}
	t, err := time.Parse(time.RFC3339Nano, results[0].Start)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to parse time from %s", results[0].Start)
	}
	return float64(t.Unix()), nil

}

// findSubtestLastTimelineValue returns the last recorder timeline value (seconds)
// from a test in the results-chart.json result.
func findSubtestLastTimelineValue(subtestDir string) (float64, error) {
	var resultsDict map[string]interface{}
	jsonData, err := os.ReadFile(filepath.Join(subtestDir, "results-chart.json"))
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to read results-chart.json")
	}
	if err := json.Unmarshal(jsonData, &resultsDict); err != nil {
		return 0.0, errors.Wrap(err, "failed to parse results-chart.json")
	}

	var timelineValues []interface{}
	if resultsDict["Power.t"] != nil {
		map1 := resultsDict["Power.t"].(map[string]interface{})
		map2 := map1["summary"].(map[string]interface{})
		timelineValues = map2["values"].([]interface{})
	} else if resultsDict["t"] != nil {
		map1 := resultsDict["t"].(map[string]interface{})
		map2 := map1["summary"].(map[string]interface{})
		timelineValues = map2["values"].([]interface{})
	} else {
		return 0.0, errors.New("no timeline data in original test")
	}

	lastTimelineValue := timelineValues[len(timelineValues)-1].(float64)
	return lastTimelineValue, nil
}

// TrimSubtestResults creates a deep copy of servod test perf.Values trimmed to the duration of the subtest.
func TrimSubtestResults(ctx context.Context, resultsDir, subtest string, values *perf.Values) (*perf.Values, error) {
	subtestDir := filepath.Join(resultsDir, "tests", subtest)
	measureStarted, err := findSubtestStartTime(resultsDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get subtest start time")
	}

	lastTimelineValue, err := findSubtestLastTimelineValue(subtestDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get last timeline value")
	}

	measureEnded := measureStarted + lastTimelineValue

	// Get perf metric by name.
	var servoIntervalMetric perf.Metric
	for metric := range values.GetValues() {
		if metric.Name == ServoIntervalMetricName {
			servoIntervalMetric = metric
			break
		}
	}
	if servoIntervalMetric.Name != ServoIntervalMetricName {
		return nil, errors.New("couldn't find servod interval metric")
	}

	// Get start and end index of timestamps that overlap with subtest.
	intervalData := values.GetValueByMetric(servoIntervalMetric)
	overlapStartIdx := 0
	overlapEndIdx := len(intervalData)
	for index, value := range intervalData {
		if value >= measureStarted {
			overlapStartIdx = index
			break
		}
	}

	for index := overlapStartIdx + 1; index < overlapEndIdx; index++ {
		if intervalData[index] > measureEnded {
			overlapEndIdx = index
			break
		}
	}

	if overlapStartIdx == overlapEndIdx {
		return nil, errors.New("no data overlap with servo and subtest")
	}

	// Create deep copy with trimmed values to match subtest.
	pv := perf.NewValues()
	for metric, value := range values.GetValues() {
		if metric.Name == IntervalMetricName {
			// Skip these timestamps as they are not used.
			continue
		}
		if metric.Name == ServoIntervalMetricName {
			intervalMetric := perf.Metric{
				Name:      IntervalMetricName,
				Unit:      "s",
				Multiple:  true,
				Direction: perf.SmallerIsBetter,
			}
			first := intervalData[overlapStartIdx]
			for i := overlapStartIdx; i < overlapEndIdx; i++ {
				pv.Append(intervalMetric, value[i]-first)
			}
		} else {
			pv.Append(metric, value[overlapStartIdx:overlapEndIdx]...)
		}
	}
	return pv, nil
}
