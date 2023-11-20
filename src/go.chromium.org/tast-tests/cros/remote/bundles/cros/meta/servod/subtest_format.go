// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package servod includes util functions for ServodWrapper.
package servod

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/errors"
)

// SubtestResult is the mapping to the results.json object.
type SubtestResult struct {
	Name  string `json:"name"`
	Start string `json:"start"`
}

// FindSubtestStartTime gets the start timestamp (seconds since January 1, 1970)
// of a test from the results.json file.
func FindSubtestStartTime(subtestDir string) (float64, error) {
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

// FindSubtestLastTimelineValue returns the last recorder timeline value (seconds)
// from a test in the results-chart.json result.
func FindSubtestLastTimelineValue(subtestDir string) (float64, error) {
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

	var lastTimelineValue float64
	for _, v := range timelineValues {
		lastTimelineValue = v.(float64)
	}
	return lastTimelineValue, nil
}
