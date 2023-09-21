// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// listVtsTests lists all test cases in a VTS executable.
// TODO(crbug.com/946390): Migrate to gtest package once it supports ARC.
func listVtsTests(ctx context.Context, a *ARC, exec string) ([]string, error) {
	output, err := a.Command(ctx, exec, "--gtest_list_tests").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	return parseVtsTestList(string(output)), nil
}

// parseVtsTestList parses the output of --gtest_list_tests and returns the
// list of test names. The format of output should be:
//
// TestSuiteA.
//
//	TestCase1
//	TestCase2
//
// TestSuiteB.
//
//	TestCase3
//	TestCase4
//
// etc. The each returned test name is formatted into "TestSuite.TestCase".
func parseVtsTestList(content string) []string {
	var suite string
	var result []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, " ") {
			// Test case name.
			result = append(result, fmt.Sprintf("%s%s", suite, strings.TrimSpace(line)))
		} else {
			// Test suite name. Note: suite contains a trailing period.
			suite = strings.TrimSpace(line)
		}
	}
	return result
}

// runVtsCase executes the specified testcase. Both stdout and stderr will be
// redirected to logfile.
func runVtsCase(ctx context.Context, exec, testcase, logfile string) error {
	// Ensure the log directory exists.
	if err := os.MkdirAll(filepath.Dir(logfile), 0755); err != nil {
		return err
	}

	// Create the output file that the test log is dumped on failure.
	f, err := os.Create(logfile)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := BootstrapCommand(ctx, exec, "--gtest_filter="+testcase)
	cmd.Stdout = f
	cmd.Stderr = f
	return cmd.Run()
}

// RunVtsTests executes all test cases in a VTS executable.
func RunVtsTests(ctx context.Context, a *ARC, testExecLocalPath, outDir string) (func(), error) {
	testing.ContextLog(ctx, "Pushing test binary to ARC: ", testExecLocalPath)
	testExecPath, err := a.PushFileToTmpDir(ctx, testExecLocalPath)
	if err != nil {
		return func() {}, errors.Wrap(err, "failed to push test binary to ARC")
	}
	cleanup := func() {
		a.Command(ctx, "rm", testExecPath).Run()
	}

	if err := a.Command(ctx, "chmod", "0777", testExecPath).Run(testexec.DumpLogOnError); err != nil {
		return cleanup, errors.Wrap(err, "failed to change test binary permissions")
	}

	testing.ContextLog(ctx, "Running tests")

	testCases, err := listVtsTests(ctx, a, testExecPath)
	if err != nil {
		return cleanup, errors.Wrap(err, "failed to list test cases")
	}
	logdir := filepath.Join(outDir, "gtest")

	for _, testCase := range testCases {
		testing.ContextLog(ctx, "Running ", testCase)

		logfile := filepath.Join(logdir, testCase+".log")
		if err := runVtsCase(ctx, testExecPath, testCase, logfile); err != nil {
			return cleanup, errors.Wrapf(err, "%s failed", testCase)
		}
	}
	return cleanup, nil
}
