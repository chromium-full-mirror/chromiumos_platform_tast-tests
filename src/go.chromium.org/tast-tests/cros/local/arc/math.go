// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"math"

	"go.chromium.org/tast/core/errors"
)

// ScoreList defines a collection of scores for calculations.
type ScoreList []float64

// CalcGeometricMean computes the geometric mean but use antilog method to
// prevent overflow: EXP((LOG(x1) + LOG(x2) + LOG(x3)) ... + LOG(xn)) / n)
func CalcGeometricMean(scores ScoreList) (float64, error) {
	if len(scores) == 0 {
		return 0, errors.New("scores can not be empty")
	}

	var mean float64
	for _, score := range scores {
		mean += math.Log(score)
	}
	mean /= float64(len(scores))

	return math.Exp(mean), nil
}

// CalcArithmeticMean returns the arithmetic mean of the passed scores.
func CalcArithmeticMean(scores ScoreList) (float64, error) {
	if len(scores) == 0 {
		return 0, errors.New("scores can not be empty")
	}

	var sum float64
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores)), nil
}
