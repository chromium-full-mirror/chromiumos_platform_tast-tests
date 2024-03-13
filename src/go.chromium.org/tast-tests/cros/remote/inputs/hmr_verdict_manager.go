// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

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
	return analyzeLinearity(stationaryPointsRemoved)
}
