// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"fmt"
	"math"

	"go.chromium.org/tast/core/errors"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
)

// analyzeLinearity calculates linear fit for a single swipe line. It
// validates max errors, root mean squared error and angle error.
func analyzeLinearity(points []*hmrNode) ([]ValidationResult, error) {
	var linError []float64
	var linErrorMax, linErrorRms, angleError float64

	if len(points) == 0 {
		return []ValidationResult{}, errors.New("no points to analyse")
	}
	x := make([]float64, len(points))
	y := make([]float64, len(points))
	for i, point := range points {
		x[i] = point.coorX
		y[i] = point.coorY
	}
	// Calculates standard form of linear equation
	// ax + by + c = 0
	c, a := stat.LinearRegression(x, y, nil, false)

	// TODO: b/314207740 - Transform the csv data from pixel location into mm with the start point as the origin to prevent the need for this workaround.
	// Linear Regression can fail, and return NaN values if the line is directly parallel to the y axis.
	if math.IsNaN(c) || math.Abs(a) > 1.0 {
		x, y = y, x
		c, a = stat.LinearRegression(x, y, nil, false)
	}
	b := -1.0

	// Point y-coordinates in the "fitted line coordinate frame" where the fitted line is the x-axis.
	// These are signed values, not unsigned distances.
	fittedY := make([]float64, len(x))
	floats.AddScaled(fittedY, a, x)
	floats.AddScaled(fittedY, b, y)
	floats.AddConst(c, fittedY)

	// Scaling factor for coordinates so that fittedY values are relative to the gradient of the dependent variable.
	scalingFactor := math.Sqrt(math.Pow(a, 2) + math.Pow(b, 2))
	floats.Scale(1/scalingFactor, fittedY)

	// Distances of points to fitted line.
	linError = elementwiseAbsolute(fittedY)

	// Max deviation from fit line to data set.
	linErrorMax = floats.Max(linError)
	// Root mean squared deviation from fit line to data set.
	linErrorRms = math.Sqrt(stat.Mean(elementwisePower(linError, 2), nil))
	// Angle from start point in data set to end point.
	angleError = math.Abs(math.Atan(a) * 180 / math.Pi)

	var errs error
	var results []ValidationResult

	result, err := validateMaxLinearity(linErrorMax)
	if err != nil {
		errs = errors.Join(errs, err)
	} else {
		results = append(results, result)
	}

	result, err = validateRMSLinearity(linErrorRms)
	if err != nil {
		errs = errors.Join(errs, err)
	} else {
		results = append(results, result)
	}

	result, err = validateAngle(angleError)
	if err != nil {
		errs = errors.Join(errs, err)
	} else {
		results = append(results, result)
	}

	if errs != nil {
		errs = errors.Wrap(errs, "Linearity Analyzer")
	}
	for i, result := range results {
		results[i].Message = fmt.Sprintf("Linearity Analyzer: %v", result.Message)
	}

	return results, errs
}

func elementwiseAbsolute(vals []float64) []float64 {
	res := make([]float64, len(vals))
	for i, val := range vals {
		res[i] = math.Abs(val)
	}
	return res
}

func elementwisePower(vals []float64, exponent float64) []float64 {
	res := make([]float64, len(vals))
	for i, val := range vals {
		res[i] = math.Pow(val, exponent)
	}
	return res
}
