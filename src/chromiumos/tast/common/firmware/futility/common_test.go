// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

// This file contains common codebase used by futility module unit-tests.

import (
	"chromiumos/tast/errors"
	"context"
	"reflect"
)

const (
	testFutilityPath = "test_futility" // Mock path to futility. Won't actually be called.
)

// testCommandRunner is simple configurable runner used to test
// the futility module implementation.
type testCommandRunner struct {
	stdout, stderr []byte
	err            error

	args []string // List of arguments from runCommandLine().
}

// newTestInstance returns new futility Instance with empty params,
// testFutilityPath as futility path, and testCommandRunner configured with
// input parameters.
func newTestInstance(stdout, stderr []byte, err error) *Instance {
	return &Instance{
		params:       Params{},
		futilityPath: testFutilityPath,
		commandRunner: &testCommandRunner{
			stdout: stdout,
			stderr: stderr,
			err:    err,
			args:   nil,
		},
	}
}

// runCommandLine implements the ContextCommandRunner interface.
func (r *testCommandRunner) runCommandLine(ctx context.Context, cmdArgs []string) (stdout, stderr []byte, err error) {
	r.args = cmdArgs
	return r.stdout, r.stderr, r.err
}

// optionalArgsMatch checks whether optional args match.
func optionalArgsMatch(inputArgs, optionalArgs []string) bool {
	if len(inputArgs) < len(optionalArgs) {
		return false
	}

	for i := range optionalArgs {
		if inputArgs[i] != optionalArgs[i] {
			return false
		}
	}
	return true
}

// assertCalledWith checks arguments stored by runCommandLine by removing
// optionalArgs, and then checking if the rest is equal to requiredArgs.
//
// Returns error on failure and nil on success.
func (r *testCommandRunner) assertCalledWith(requiredArgs []string, optionalArgs [][]string) error {
	allArgs := r.args

	// Remove all optional arguments.
	for _, opt := range optionalArgs {
		for i := 0; i < len(allArgs); i++ {
			if optionalArgsMatch(allArgs[i:], opt) {
				allArgs = append(allArgs[:i], allArgs[(i+len(opt)):]...)
				break
			}
		}
	}

	if !reflect.DeepEqual(requiredArgs, allArgs) {
		return errors.Errorf("required arguments do not match with stored, expected: %q, got %q", requiredArgs, allArgs)
	}

	return nil
}
