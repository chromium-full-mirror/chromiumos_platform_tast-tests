// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

// IgtTest is used to describe the config used to run each test.
type IgtTest struct {
	Exe              string   // The test executable name.
	Subtests         []string // The subtests to run.
	IsSkipOk         bool     // If true, the test is allowed to be skipped and report as a pass.
	DisableHangCheck bool     // If true, disable the gpu hang check as the test produces hangs intentionally.
}

// igtResultSummary is a summary of results from an igt test log.
type igtResultSummary struct {
	passed  int // number of passed subtests
	failed  int // number of failed subtests
	skipped int // number of skipped subtests
}

// IgtGpuQcom is a list of freedreno boards.
var IgtGpuQcom = []string{"strongbad", "trogdor"}

// IgtGpuMtk is a list of MTK boards.
var IgtGpuMtk = []string{"kukui", "jacuzzi", "asurada", "cherry", "corsola", "geralt"}

// IgtExecuteTests executes the IGT binary of the an IgtTest. If the test has a subtest, it executes it as well. Otherwise, it executes the entire test.
func IgtExecuteTests(ctx context.Context, testOpt IgtTest, f *os.File) (bool, *exec.ExitError, error) {
	exePath := filepath.Join("/usr/local/libexec/igt-gpu-tools", testOpt.Exe)

	var cmd *testexec.Cmd
	if len(testOpt.Subtests) > 0 {
		// IGT accepts wildcards
		subtests := strings.Join(testOpt.Subtests, ",")
		cmd = testexec.CommandContext(ctx, exePath, "--run-subtest", subtests)
	} else {
		cmd = testexec.CommandContext(ctx, exePath)
	}

	cmd.Stdout = f
	cmd.Stderr = f
	err := cmd.Run()
	exitErr, isExitErr := err.(*exec.ExitError)

	// Reset the file to the beginning so the log can be read out again.
	f.Seek(0, 0)

	return isExitErr, exitErr, err
}

func igtSummarizeLog(f *os.File) (r igtResultSummary, failedSubtests []string) {
	var igtSubtestResultRegex = regexp.MustCompile("^(?:Subtest (.*): )?([A-Z]+)")

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if m := igtSubtestResultRegex.FindStringSubmatch(scanner.Text()); m != nil {
			result := m[2]
			switch result {
			case "SKIP":
				r.skipped++
			case "FAIL":
				r.failed++
				if subtestName := m[1]; subtestName != "" {
					failedSubtests = append(failedSubtests, subtestName)
				}
			case "SUCCESS":
				r.passed++
			}
		}
	}
	return r, failedSubtests
}

// IgtProcessResults reads the results of the test output and outputs a summary of the full test results.
func IgtProcessResults(igtTest IgtTest, file *os.File, isExitErr bool, exitErr *exec.ExitError, err error) (bool, string) {
	results, failedSubtests := igtSummarizeLog(file)
	summary := fmt.Sprintf("Ran %d subtests with %d failures and %d skipped",
		results.passed+results.failed, results.failed, results.skipped)

	// In the case of running multiple subtests which all happen to be skipped, igt_exitcode is 0,
	// but the final exit code will be 77.
	if results.passed+results.failed == 0 && isExitErr && exitErr.ExitCode() == 77 {
		outputLog := fmt.Sprintf("ALL %d subtests were SKIPPED: %s\n", results.skipped, err.Error())
		// If we skip the test, return if it is allowed to be skipped.
		return !igtTest.IsSkipOk, outputLog
		// Each test is expected to run and either pass or fail. If nothing happens, then something is off.
	} else if results.passed+results.failed+results.skipped == 0 {
		outputLog := "Entire test was skipped and this is not expected - No subtests were run\n"
		return true, outputLog
	} else if len(failedSubtests) > 0 {
		outputLog := fmt.Sprintf("FAIL: Test:%s - Pass:%d Fail:%d - FailedSubtests:%s - Summary:%s\n",
			igtTest.Exe, results.passed, results.failed, failedSubtests, summary)
		return true, outputLog
	} else {
		outputLog := fmt.Sprintf("%s\n", summary)
		return false, outputLog
	}
}
