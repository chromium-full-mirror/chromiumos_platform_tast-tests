// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"fmt"
	"math"

	"go.chromium.org/tast/core/errors"
)

// Reference image is in mm by default, on a 13.3 diagonal screen.
const (
	defaultReferenceScreenHeight = 165.6
	defaultReferenceScreenWidth  = 294.4
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

	result, err = analyzeGapRatio(transformedPoints)
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

// DetermineFullImageVerdict performs a full image analysis on a HMR's CSV result file.
// It does this by performing a comparison between the result file, and the reference file (the CSV file used to generate the gcode file run on the HMR).
// The reference file is scaled to mm on a 13.3 inch diagnonal screen. The result file is raw unscaled data scaled in pixels, taken directly from the DUT.
func DetermineFullImageVerdict(referenceFileName, resultFileName string, calibrationData CalibrationData, resultScreenWidth, resultScreenHeight float64) (*FullImageResult, error) {
	referencePoints, err := readCSV(referenceFileName)
	if err != nil {
		return nil, err
	}
	resultPoints, err := readCSV(resultFileName)
	if err != nil {
		return nil, err
	}

	// calibrationData contains information regarding known data errors in this HMR/DUT setup.
	// As the result image was captured via this HMR setup, the data errors will be present in it.
	// As the reference image was generated manually by hand and not in this HMR/DUT setup, the data errors will not be present.
	// Therefore these data errors should manually be applied to the reference image, so that the images align.
	referenceDerotated := applyRotation(referencePoints, calibrationData.RotationError)

	referenceDerotatedDeskewed := applySkew(referenceDerotated,
		calibrationData.SkewErrorRelativeToScreenWidth*defaultReferenceScreenWidth,
		defaultReferenceScreenHeight)

	// Scales reference image to be same size as result image's screen.
	referenceDerotatedDeskewedScaled := applyScale(referenceDerotatedDeskewed,
		resultScreenWidth/defaultReferenceScreenWidth,
		resultScreenHeight/defaultReferenceScreenHeight)
	referenceDerotatedDeskewedScaledOffset := applyOffset(referenceDerotatedDeskewedScaled,
		calibrationData.OffsetX,
		calibrationData.OffsetY)

	// Scales result image from pixels to mm.
	resultScaled := applyScale(resultPoints, calibrationData.ScaleFactorX, calibrationData.ScaleFactorY)
	resultScaledOffset := applyOffset(resultScaled, calibrationData.OffsetX, calibrationData.OffsetY)

	referencePaths := extractReferencePaths(referenceDerotatedDeskewedScaledOffset)
	if len(referencePaths) == 0 {
		return nil, errors.Errorf("no reference paths could be extracted from %v", referenceFileName)
	}

	screenDiagonalDistance := math.Sqrt(math.Pow(resultScreenWidth, 2) + math.Pow(resultScreenHeight, 2))
	resultPaths, epsilon, err := extractResultsPaths(resultScaledOffset, referencePaths, screenDiagonalDistance)
	if err != nil {
		return nil, err
	}

	// Note: Temporarily required, as tast cannot be built with unused variables. Will be removed in next commit.
	_, _, _ = referencePaths, resultPaths, epsilon
	// TODO(b/343548793): Run sliding window analysis on reference and result paths.
	return nil, nil
}
