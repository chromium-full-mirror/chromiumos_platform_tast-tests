// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast/core/errors"
)

// SubtestResult is the mapping to the results.json object.
type SubtestResult struct {
	Name  string `json:"name"`
	Start string `json:"start"`
}

// findSubtestStartTime gets the start timestamp (seconds since January 1, 1970)
// of a test from the perf results.json file.
func findSubtestStartTime(subtestDir string) (time.Time, error) {
	// TODO: b/314232534 - Use start timestamp in power_log.json
	var results []SubtestResult
	rf, err := os.Open(filepath.Join(subtestDir, "results.json"))
	if err != nil {
		return time.Time{}, errors.Wrap(err, "couldn't open results file")
	}
	defer rf.Close()

	if err = json.NewDecoder(rf).Decode(&results); err != nil {
		return time.Time{}, errors.Wrapf(err, "couldn't decode results from %v", rf.Name())
	}
	if len(results) > 1 {
		return time.Time{}, errors.New("more than one subtest was run")
	}
	return time.Parse(time.RFC3339Nano, results[0].Start)
}

// findSubtestLastTimelineValue returns the duration of the last timeline value
// (seconds since test started) from a test, by reading the results-chart.json timeline field.
func findSubtestLastTimelineValue(subtestDir string) (time.Duration, error) {
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
	return time.Duration(lastTimelineValue * float64(time.Second)), nil
}

// GetOverlapIndices determines the start and end indices given a start time and timestamp dataset
// that overlap with the specified measurement period.
func GetOverlapIndices(measureStarted, measureEnded time.Time, tsData []time.Time) (int, int, error) {
	overlapStartIdx := 0
	for i, ts := range tsData {
		overlapStartIdx = i
		if ts.Equal(measureStarted) || ts.After(measureStarted) {
			break
		}
	}

	overlapEndIdx := overlapStartIdx
	for i := overlapStartIdx; i < len(tsData); i++ {
		overlapEndIdx = i
		if tsData[i].Equal(measureEnded) || tsData[i].After(measureEnded) {
			break
		}
	}

	if overlapStartIdx == overlapEndIdx {
		return 0, 0, errors.New("no data overlap with subtest")
	}
	return overlapStartIdx, overlapEndIdx, nil
}

// TrimSubtestResults takes the perf values of and the name of the subtest whose timeline
// duration we want to match. It reads the start and end timestamp (seconds since January 1, 1970)
// of the subtest, and trims remote side perf values to the duration of the subtest.
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
	measureEnded := measureStarted.Add(lastTimelineValue)

	// Get perf metric by name.
	var intervalMetric perf.Metric
	for metric := range values.GetValues() {
		if metric.HasStartTs {
			intervalMetric = metric
			break
		}
	}

	if !intervalMetric.HasStartTs {
		return nil, errors.New("couldn't find valid interval metric")
	}

	// Get start and end index of timestamps that overlap with subtest.
	tsData, err := values.ConvertToUnixTss(intervalMetric)
	if err != nil || len(tsData) == 0 {
		return nil, errors.Wrap(err, "couldn't find valid timestamp data")
	}

	overlapStartIdx, overlapEndIdx, err := GetOverlapIndices(measureStarted, measureEnded, tsData)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't find overlap with subtest")
	}

	// Trim start and end values to subtest timing only.
	for key, vs := range values.GetValues() {
		values.GetValues()[key] = vs[overlapStartIdx : overlapEndIdx+1]
	}

	// Shift timestamp values relative to the subtest start.
	intervalData := values.GetValues()[intervalMetric]
	for i, ts := range intervalData {
		ds := time.Duration(ts * float64(time.Second))
		intervalData[i] = float64(intervalMetric.StartTs.Add(ds).Sub(measureStarted).Nanoseconds()) / 1000000000.0
	}
	intervalMetric.StartTs = measureStarted

	return values, nil
}
