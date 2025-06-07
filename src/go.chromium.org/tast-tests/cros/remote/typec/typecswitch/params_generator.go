// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch fixture and helper functions for the tests in the typec directory.
package typecswitch

import (
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast/core/testing"
)

const (
	stressMultiplier = 5
)

// GenerateParams generates parameters for the typec legacy tests.
func GenerateParams(timeout, iterations int, mode usbswitch.ConnectionMode, attr string) []testing.Param {
	timeoutNormal := time.Duration(timeout) * time.Minute
	timeoutStress := time.Duration(timeout) * stressMultiplier * time.Minute
	iterationsStress := iterations * stressMultiplier

	return []testing.Param{{
		ExtraAttr: []string{attr, "typec_unigraf274"},
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterations,
		},
		Timeout: timeoutNormal,
	}, {
		Name:      "flipped",
		ExtraAttr: []string{"typec_unigraf274"},
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterations,
			Flipped:        true,
		},
		Timeout: timeoutNormal,
	}, {
		Name: "stress",
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterationsStress,
		},
		Timeout: timeoutStress,
	}, {
		Name: "stress_flipped",
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterationsStress,
			Flipped:        true,
		},
		Timeout: timeoutStress,
	}}
}
