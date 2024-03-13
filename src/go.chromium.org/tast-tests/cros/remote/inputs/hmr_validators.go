// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"fmt"
	"math"

	"go.chromium.org/tast/core/errors"
)

type comparatorType string

const (
	le comparatorType = "<="
	ge comparatorType = ">="
	eq comparatorType = "=="
)

// ValidationResult is the result of a HMR validation. Message provides additional context as to why a validation passed/failed.
type ValidationResult struct {
	Passed  bool
	Message string
}

func validate(value, limit, tolerance float64, comparator comparatorType) (ValidationResult, error) {
	var deviance, score float64
	switch comparator {
	case le:
		deviance = value - limit
	case ge:
		deviance = limit - value
	case eq:
		deviance = math.Abs(value - limit)
	default:
		return ValidationResult{Passed: false, Message: ""}, errors.Errorf("invalid comparator (%v)", comparator)
	}

	if deviance <= 0 {
		score = 1.0
	} else if deviance > tolerance {
		score = 0.0
	} else {
		score = (tolerance - deviance) / tolerance
	}

	if score < 0 || score > 1 {
		return ValidationResult{Passed: false, Message: ""}, errors.Errorf("invalid score (%v)", score)
	}

	if score == 0 {
		failedMessage := fmt.Sprintf("failed with score: (0), value: (%v) deviance: (%v) limit: (%v) tolerance (%v) comparator (%v)", value, deviance, limit, tolerance, comparator)
		return ValidationResult{Passed: false, Message: failedMessage}, nil
	}

	successMessage := fmt.Sprintf("passed with score: (%v), value: (%v) deviance: (%v) limit: (%v) tolerance (%v) comparator (%v)", score, value, deviance, limit, tolerance, comparator)
	return ValidationResult{Passed: true, Message: successMessage}, nil
}

func validateMaxLinearity(value float64) (ValidationResult, error) {
	result, err := validate(value, 2, 1, le)
	if err != nil {
		return result, errors.Wrap(err, "Max Linearity Validation")
	}
	result.Message = fmt.Sprintf("Max Linearity Validation: %v", result.Message)
	return result, nil
}
