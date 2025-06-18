// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecunigraf contains fixtures and setup utilities for Unigraf device testing.
package typecunigraf

import (
	"fmt"
	"time"

	"go.chromium.org/tast/core/testing"
)

// GenerateUnigrafParams generates parameters for Unigraf-based Type-C tests.
// It creates parameter sets for normal and flipped orientations on Unigraf ports 0 and 1.
func GenerateUnigrafParams(
	unigrafSetupData TestSetupData,
	timeoutMinutes int,
	attrs ...string,
) []testing.Param {
	var params []testing.Param

	timeout := time.Duration(timeoutMinutes) * time.Minute

	// Iterate over Unigraf ports (0 and 1) and orientations (normal and flipped)
	for _, portNum := range []int{0, 1} {
		for _, flipped := range []bool{false, true} {
			orientationStr := "normal"
			if flipped {
				orientationStr = "flipped"
			}

			// Add "typec_unigraf274" attr only for port 0 tests
			var defaultParams []string
			if portNum == 0 {
				defaultParams = append(defaultParams, "typec_unigraf274")
			}

			unigrafSetupData.PortNum = portNum
			unigrafSetupData.Flipped = flipped

			params = append(params, testing.Param{
				Name:      fmt.Sprintf("port%d_%s", portNum, orientationStr),
				ExtraAttr: append(defaultParams, attrs...),
				Val:       unigrafSetupData,
				Timeout:   timeout,
			})
		}
	}
	return params
}
