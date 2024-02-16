// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"testing"
)

func TestSucceededAfterRetry(t *testing.T) {
	rl := &Loop{Attempts: 1,
		MaxAttempts: 3,
		DoRetries:   true,
		Errorf:      t.Fatalf,
		Logf:        func(format string, args ...interface{}) {},
	}

	rl.Retry("first attempt", nil)
	rl.Retry("second attempt", nil)
	// Assume that the third attempt succeeded.

	// rl.Errorf was never called.
}

func TestAllAttemptsFailedWithRetry(t *testing.T) {
	numErrorfCalled := 0
	expectedNumErrorfCalled := 3

	rl := &Loop{Attempts: 1,
		MaxAttempts: 3,
		DoRetries:   true,
		Errorf: func(format string, args ...interface{}) {
			numErrorfCalled++
		},
		Logf: func(format string, args ...interface{}) {},
	}

	rl.Retry("first attempt", nil)
	rl.Retry("second attempt", nil)
	rl.Retry("third attempt", nil)

	if numErrorfCalled != expectedNumErrorfCalled {
		t.Errorf("numErrorfCalled mismatch: expected %d, got %d", expectedNumErrorfCalled, numErrorfCalled)
	}
}

func TestExitAfterRetry(t *testing.T) {
	numErrorfCalled := 0
	expectedNumErrorfCalled := 2

	rl := &Loop{Attempts: 1,
		MaxAttempts: 3,
		DoRetries:   true,
		Errorf: func(format string, args ...interface{}) {
			numErrorfCalled++
		},
		Logf: func(format string, args ...interface{}) {},
	}

	rl.Retry("first attempt", nil)
	rl.Exit("second attempt", nil)

	if numErrorfCalled != expectedNumErrorfCalled {
		t.Errorf("numErrorfCalled mismatch: expected %d, got %d", expectedNumErrorfCalled, numErrorfCalled)
	}
}
