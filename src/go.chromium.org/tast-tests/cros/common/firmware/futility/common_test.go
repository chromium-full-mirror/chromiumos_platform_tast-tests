// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

// This file contains common codebase used by futility module unit-tests.

import (
	"context"
	"reflect"

	"go.chromium.org/tast/core/errors"
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

// argsSubsliceMatch checks whether patternArgs are sub-slice of inputArgs.
func argsSubsliceMatch(inputArgs, patternArgs []string) bool {
	if len(inputArgs) < len(patternArgs) {
		return false
	}

	for i := range patternArgs {
		if inputArgs[i] != patternArgs[i] {
			return false
		}
	}
	return true
}

// assertCalledWith checks arguments stored by runCommandLine.
//
//  1. Remove all extraArgs arguments list, as they are not mandatory.
//  2. Try to match and remove all optionArgs arguments from arguments list.
//  3. Match the rest in order with positionalArgs.
//
// Example usage in test:
//
//	// Execute futility (in command).
//	r.runCommandLine(context.Background(),
//		[]string{"futility", "gbb",
//			"--get", "--flags",
//			"--servo_port", "9999", "output.bin"})
//
//	// Create list of positional, option (NOT OPTIONAL)
//	// and extra (optional, not mandatory).
//	positionalArgs := []string{"futility", "gbb"}
//	optionArgs := [][]string{{"--get"}, {"--flags"},
//		                     {"--hwid"}, {"--servo_port", "9999"}}
//	extraArgs := [][]string{{"--servo"}}
//	err := r.assertCalledWith(positionalArgs, optionArgs, extraArgs)
//
//	// err will contain:
//	//   option argument not matched: ["--hwid"]
//	//     required arguments do not match with stored,
//	//     expected: ["futility" "gbb"],
//	//     got ["futility" "gbb" "output.bin"]
//
// Returns nil on success and with error on failure.
func (r *testCommandRunner) assertCalledWith(positionalArgs []string, optionArgs, extraArgs [][]string) error {
	allArgs := r.args
	var argsErrors []error

	// Remove all extra arguments.
	for _, opt := range extraArgs {
		for i := 0; i < len(allArgs); i++ {
			if argsSubsliceMatch(allArgs[i:], opt) {
				allArgs = append(allArgs[:i], allArgs[(i+len(opt)):]...)
				break
			}
		}
	}

	optionArgsFound := make([]bool, len(optionArgs))
	for k, opt := range optionArgs {
		for i := 0; i < len(allArgs); i++ {
			if argsSubsliceMatch(allArgs[i:], opt) {
				allArgs = append(allArgs[:i], allArgs[(i+len(opt)):]...)
				optionArgsFound[k] = true // Mark as found.
				break
			}
		}
	}
	// Check if all optionArgs matched.
	for i, v := range optionArgsFound {
		if v == false {
			argsErrors = append(argsErrors, errors.Errorf("option argument not matched: %q", optionArgs[i]))
		}
	}

	if !reflect.DeepEqual(positionalArgs, allArgs) {
		argsErrors = append(argsErrors, errors.Errorf("required arguments do not match with stored, expected: %q, got %q", positionalArgs, allArgs))
	}

	if len(argsErrors) > 0 {
		return errors.Join(argsErrors...)
	}

	return nil
}
