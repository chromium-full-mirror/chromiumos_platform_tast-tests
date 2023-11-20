// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseDiagOutputV2(t *testing.T) {
	cases := []struct {
		rawInput string
		expected RoutineResultV2
	}{
		// Passed routine without output.
		{
			"\rRunning Progress: 0\rWaiting: kWaitingToBeScheduled\n\rRunning Progress: 0\rRunning Progress: 99\rRunning Progress: 100\nStatus: Passed\n",
			RoutineResultV2{
				Progress: 100,
				Status:   StatusPassed,
			},
		},
		// Failed routine without output.
		{
			"\rRunning Progress: 0\rWaiting: kWaitingToBeScheduled\n\rRunning Progress: 0\rRunning Progress: 99\rRunning Progress: 100\nStatus: Failed\n",
			RoutineResultV2{
				Progress: 100,
				Status:   StatusFailed,
			},
		},
		// Error.
		{
			"Waiting: kWaitingToBeScheduled\n\nError: {\"error\":\"3\",\"message\":\"Sample test error\"}",
			RoutineResultV2{
				Progress: 0,
				Status:   StatusError,
				Output:   "{\"error\":\"3\",\"message\":\"Sample test error\"}",
			},
		},
		// Passed routine with output.
		{
			"\rRunning Progress: 0\rWaiting: kWaitingToBeScheduled\n\rRunning Progress: 0\rRunning Progress: 99\rRunning Progress: 100\nStatus: Passed\nOutput: {\"bytes_tested\":\"4096\"}",
			RoutineResultV2{
				Progress: 100,
				Status:   StatusPassed,
				Output:   "{\"bytes_tested\":\"4096\"}",
			},
		},
	}
	for _, c := range cases {
		got, err := parseDiagOutputV2(context.Background(), c.rawInput)
		if err != nil {
			t.Error("error in parseDiagOutput: ", err)
		}
		if diff := cmp.Diff(c.expected, got); diff != "" {
			t.Error("parse diag test failed (-expected + got): ", diff)
		}
	}
}
