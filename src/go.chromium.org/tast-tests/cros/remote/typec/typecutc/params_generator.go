// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecutc contains fixtures and setup utilities for utc device testing.
package typecutc

import (
	"fmt"
	"time"

	"go.chromium.org/tast/core/testing"
)

// GenerateUtcParams generates parameters for utc-based Type-C tests.
// It creates parameter sets for normal and flipped orientations on utc ports 0 and 1.
func GenerateUtcParams(
	utcSetupData TestSetupData,
	timeoutMinutes int,
	attrs ...string,
) []testing.Param {
	var params []testing.Param

	timeout := time.Duration(timeoutMinutes) * time.Minute

	// Iterate over utc ports (0 and 1) and orientations (normal and flipped)
	for _, portNum := range []int{0, 1} {
		// TODO(b/434628173) Unblock flipped tests once the bug is fixed.
		for _, flipped := range []bool{false} {
			orientationStr := "normal"
			if flipped {
				orientationStr = "flipped"
			}

			// Add "typec_utc274" attr only for port 0 tests
			var defaultParams []string
			if portNum == 0 {
				defaultParams = append(defaultParams, "typec_utc274")
			}

			utcSetupData.PortNum = portNum
			utcSetupData.Flipped = flipped

			params = append(params, testing.Param{
				Name:      fmt.Sprintf("port%d_%s", portNum, orientationStr),
				ExtraAttr: append(defaultParams, attrs...),
				Val:       utcSetupData,
				Timeout:   timeout,
			})
		}
	}
	return params
}
