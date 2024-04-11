// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cleanupchecks

import (
	"context"
	"strconv"

	cs "go.chromium.org/tast-tests/cros/services/cros/cleanupchecks"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CleanUpChecksLevel is the level of clean up checks we want to execute in
// terms of time taken by the checks.
var CleanUpChecksLevel = testing.RegisterVarString(
	"cleanupchecks.Level",
	"0",
	"Variable that sets the level of clean up checks to be run",
)

// LevelEnum is used for the level of clean up checks
// that will be executed.
type LevelEnum int

const (
	// Disabled disables the check.
	Disabled = iota
	// Fast are checks that are executed in under 1 second.
	Fast
	// Medium are checks that are executed in under 5 seconds.
	Medium
	// Slow are checks that are executed in 5 or more seconds.
	Slow
)

// CleanUpCheck is a struct that defines a single clean up check.
type CleanUpCheck struct {
	Check       func(context.Context, *cs.PreTestData) error
	RecordState func(context.Context) (*cs.PreTestData, error)
	Level       LevelEnum
	Enabled     bool
}

// RecordStateBeforeTest collects data of the DUT state before the test is run.
func RecordStateBeforeTest(ctx context.Context, req *cs.RecordStateBeforeTestFromDUTRequest) (*cs.RecordStateBeforeTestFromDUTResponse, error) {
	var allErrors error = nil
	stateDataMap := map[string]*cs.PreTestData{}
	level, err := strconv.Atoi(CleanUpChecksLevel.Value())
	if err != nil {
		return nil, errors.Wrapf(err, "invalid level: %s for clean up checks", CleanUpChecksLevel.Value())
	}
	for checkName, check := range listOfAllChecks {
		if !check.Enabled || int(check.Level) > level {
			continue
		}
		if check.RecordState != nil {
			stateData, err := check.RecordState(ctx)
			if err != nil {
				allErrors = errors.Join(allErrors, errors.Wrapf(err, "%s: ", checkName))
			}
			stateDataMap[checkName] = stateData
		}
	}

	return &cs.RecordStateBeforeTestFromDUTResponse{PreTestDataMap: stateDataMap}, allErrors
}

// ExecuteCleanUpChecks executes all enabled clean up checks.
func ExecuteCleanUpChecks(ctx context.Context, data *cs.ExecuteCleanUpChecksOnDUTRequest) (*cs.ExecuteCleanUpChecksOnDUTResponse, error) {
	var allErrors error = nil
	level, err := strconv.Atoi(CleanUpChecksLevel.Value())
	if err != nil {
		return nil, errors.Wrapf(err, "invalid level: %s for clean up checks", CleanUpChecksLevel.Value())
	}

	for checkName, check := range listOfAllChecks {
		if !check.Enabled || int(check.Level) > level {
			continue
		}
		err = check.Check(ctx, data.PreTestDataMap[checkName])
		if err != nil {
			allErrors = errors.Join(allErrors, errors.Wrapf(err, "%s: ", checkName))
		}
	}
	return &cs.ExecuteCleanUpChecksOnDUTResponse{}, allErrors
}
