// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package servod includes util functions for ServodWrapper.
package servod

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast/core/errors"
)

// FindSubtestStartTime gets first occurring timestamp (seconds since January 1, 1970)
// of a test from the log.txt result.
func FindSubtestStartTime(subtestDir string) (float64, error) {
	logPath := filepath.Join(subtestDir, "log.txt")
	log, err := os.ReadFile(logPath)
	if err != nil {
		return 0.0, errors.Wrapf(err, "failed to read %q", logPath)
	}
	// TODO: b/303755525 - Change method of seeking test start time.
	re := regexp.MustCompile("(.*Z) .* Start tracking zram IO stats")
	match := re.FindStringSubmatch(string(log))
	if len(match) > 1 {
		t, err := time.Parse(time.RFC3339Nano, match[1])
		if err != nil {
			return 0.0, errors.Wrapf(err, "failed to parse time from %s", match[1])
		}
		return float64(t.Unix()), nil
	}

	return 0.0, errors.New("no start time found")
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
