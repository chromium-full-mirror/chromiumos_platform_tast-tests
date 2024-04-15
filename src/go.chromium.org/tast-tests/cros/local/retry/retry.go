// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Loop is a representation of retry loop state for a test.
type Loop struct {
	Attempts    int
	MaxAttempts int
	DoRetries   bool
	errors      []error
	Errorf      func(format string, args ...interface{})
	Logf        func(format string, args ...interface{})
}

// Exit ends the retry loop. This is used in places where the failure is in the core feature.
func (rl *Loop) Exit(desc string, err error) error {
	rl.errors = append(rl.errors, errors.Wrap(err, "failed to "+desc))
	for i, prevErr := range rl.errors {
		rl.Errorf("Failed attempt #%d: %s", i+1, prevErr)
	}
	return testing.PollBreak(errors.New("failed all attempts"))
}

// MustRetry retries the loop even if retries are disabled. This is used for unrelated failures.
func (rl *Loop) MustRetry(desc string, err error) error {
	if rl.Attempts < rl.MaxAttempts {
		rl.Attempts++
		err = errors.Wrap(err, "failed to "+desc)
		rl.errors = append(rl.errors, err)
		rl.Logf("%s. Retrying", err)
		return err
	}
	return rl.Exit(desc, err)
}

// Retry retries the loop. This is used for temporary retires to stabilize the test.
func (rl *Loop) Retry(desc string, err error) error {
	if rl.DoRetries {
		return rl.MustRetry(desc, err)
	}
	return rl.Exit(desc, err)
}
