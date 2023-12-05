// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hibernate

import (
	"strconv"

	"go.chromium.org/tast/core/testing"
)

const (
	// VarEmail specifies the email address of the account that should be used for testing.
	VarEmail                 = "email"
	// VarPassword specifies the password address of the account used for testing.
	VarPassword              = "password"
	// VarSimulateMemPressureMB specified the amount of memory pressure (in MB) that the test should simulate.
	VarSimulateMemPressureMB = "simulateMemPressureMB"
)

const (
	simulateMemPressureMBDefault = 0
)

// TestParameters contains parameters that are common for all hibernate tests.
type TestParameters struct {
	UserEmail string
	UserPassword string
	MemoryPressure uint32
}

// GetTestParams parses test parameters that are common to all hibernate tests.
func GetTestParams(s *testing.State) TestParameters {
	var params TestParameters

	params.UserEmail, _ = s.Var(VarEmail)
	params.UserPassword, _ = s.Var(VarPassword)

	if sval, ok := s.Var(VarSimulateMemPressureMB); ok {
		val, err := strconv.ParseUint(sval, 10, 32)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", VarSimulateMemPressureMB, sval)
		}
		if params.MemoryPressure > 32000 {
			s.Fatalf("Invalid numeric value provided for %s : %d", VarSimulateMemPressureMB, params.MemoryPressure)
		}

		params.MemoryPressure = uint32(val)
	} else {
		params.MemoryPressure = simulateMemPressureMBDefault
	}

	return params
}
