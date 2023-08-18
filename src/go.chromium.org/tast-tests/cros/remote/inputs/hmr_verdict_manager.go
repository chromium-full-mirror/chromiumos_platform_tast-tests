// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

// DetermineSingleLineVerdict runs analysis over a HMR CSV output file and returns the result of all validations run.
func DetermineSingleLineVerdict(fileName string) ([]ValidationResult, error) {
	unscaledPoints, err := readCSV(fileName)
	if err != nil {
		return []ValidationResult{}, err
	}
	// TODO: b/314207740 - Transform the pixel data retrieved from the HMR CSV into distance data in mm.

	initialTouchRemovedPoints := removeInitialTouch(unscaledPoints, 196.0)
	stationaryPointsRemoved := removeStationaryPoints(initialTouchRemovedPoints, 128.0)
	return analyzeLinearity(stationaryPointsRemoved)
}
