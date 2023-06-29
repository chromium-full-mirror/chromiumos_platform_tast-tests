// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package benchmarkcuj

import (
	"math"

	"go.chromium.org/tast/core/errors"
)

// getTValue will return the t value for degrees of freedom 9 and above.
func getTValue(df int) (float64, error) {
	if df < 1 {
		return 0, errors.New("degrees of freedom is less than 1")
	}
	// When the sample size reaches 30, we will do a normal approximation.
	if df >= 30 {
		return 1.96, nil
	}

	return []float64{12.71, 4.3, 3.18, 2.78, 2.57, 2.45,
		2.37, 2.31, 2.26, 2.23, 2.20, 2.18, 2.16, 2.14,
		2.13, 2.12, 2.11, 2.10, 2.09, 2.09, 2.08, 2.07,
		2.07, 2.06, 2.06, 2.06, 2.05, 2.05, 2.05}[df-1], nil
}

// getDelta returns the mean, sd, and 95% confidence delta.
func getDelta(samples []float64) (float64, float64, float64, error) {
	if len(samples) == 1 {
		return samples[0], 0, 0, nil
	}
	n := float64(len(samples))
	var sum, mean, sd float64
	for _, value := range samples {
		sum += value
	}
	mean = sum / n
	for _, value := range samples {
		sd += math.Pow(value-mean, 2)
	}
	sd = math.Sqrt(sd / n)

	tVal, err := getTValue(len(samples) - 1)
	if err != nil {
		return 0, 0, 0, err
	}

	return mean, sd, tVal * sd / math.Sqrt(n), nil
}
