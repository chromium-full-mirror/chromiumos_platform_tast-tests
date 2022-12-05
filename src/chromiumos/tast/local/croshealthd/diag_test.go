// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseDiagOutput(t *testing.T) {
	cases := []struct {
		rawInput string
		expected RoutineResult
	}{
		// Passed routine.
		{
			"\rProgress: 100\rProgress: 100\nStatus: Passed\nStatus message: Routine passed.\n",
			RoutineResult{
				Progress: 100,
				Status:   StatusPassed,
			},
		},
		// Report last progress.
		{
			"\rProgress: 0\rProgress: 100\rProgress: 100\nStatus: Passed\nStatus message: Routine passed.\n",
			RoutineResult{
				Progress: 100,
				Status:   StatusPassed,
			},
		},
		// Interactive routine.
		{
			"\rProgress: 25\nInteractive message.\n\rProgress: 100\rProgress: 100\nStatus: Passed\nStatus message: Routine passed.\n",
			RoutineResult{
				Progress: 100,
				Status:   StatusPassed,
			},
		},
		// Error.
		{
			"\rProgress: 0\rProgress: 20\nStatus: Error\nStatus message: Error message.\n",
			RoutineResult{
				Progress: 20,
				Status:   StatusError,
			},
		},
		// Unsupported routine.
		{
			"Status: Unsupported\nStatus Message: The routine is not supported by the device\n",
			RoutineResult{
				Progress: 0,
				Status:   StatusUnsupported,
			},
		},
	}
	for _, c := range cases {
		got, err := parseDiagOutput(context.Background(), c.rawInput)
		if err != nil {
			t.Error("error in parseDiagOutput: ", err)
		}
		if diff := cmp.Diff(&c.expected, got); diff != "" {
			t.Error("parse diag test failed (-expected + got): ", diff)
		}
	}
}
