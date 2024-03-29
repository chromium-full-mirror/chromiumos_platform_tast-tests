// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"fmt"

	"go.chromium.org/tast/core/errors"
)

// DetermineSingleLineVerdict runs analysis over a HMR CSV output file and returns the result of all validations run.
func DetermineSingleLineVerdict(fileName string, widthResolution, heightResolution float64) ([]ValidationResult, error) {
	unscaledPoints, err := readCSV(fileName)
	if err != nil {
		return []ValidationResult{}, err
	}
	// Scales points from pixels to mm.
	xScaleFactor := 1 / widthResolution
	yScaleFactor := 1 / heightResolution
	scaledPoints := applyScale(unscaledPoints, xScaleFactor, yScaleFactor)

	initialTouchRemovedPoints := removeInitialTouch(scaledPoints, 3.0)
	stationaryPointsRemoved := removeStationaryPoints(initialTouchRemovedPoints, 2.0)
	transformedPoints := transformToNormalizedCoordinates(stationaryPointsRemoved)

	var results []ValidationResult
	var errs error

	result, err := analyzeLinearity(transformedPoints)
	if err != nil {
		errs = errors.Join(errs, err)
	}
	results = append(results, result...)

	result, err = analyzeReportRate(transformedPoints)
	if err != nil {
		errs = errors.Join(errs, err)
	}
	results = append(results, result...)

	if errs != nil {
		errs = errors.Wrap(errs, "Single Line Verdict")
	}
	for i, result := range results {
		results[i].Message = fmt.Sprintf("Single Line Verdict: %v", result.Message)
	}

	return results, errs
}
