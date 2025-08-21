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

	// TODO(b/434628173) Unblock flipped tests once the bug is fixed.
	return []testing.Param{{
		ExtraAttr: []string{attr, "typec_utc274"},
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterations,
		},
		Timeout: timeoutNormal,
	}, {
		Name: "stress",
		Val: TestSetupData{
			ConnectionMode: mode,
			Iterations:     iterationsStress,
		},
		Timeout: timeoutStress,
	}}
}
