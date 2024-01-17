// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const suspendStressResults = "/tmp/suspend_results"

// SuspendStressTest performs suspend stress test for count cycles.
func SuspendStressTest(ctx context.Context, dut *dut.DUT, count int) (string, error) {
	const (
		defaultWakeMin    = " --wake_min=5 "
		defaultWakeMax    = " --wake_max=10 "
		defaultSuspendMin = " --suspend_min=5 "
		defaultSuspendMax = " --suspend_max=10 "
	)

	testing.ContextLogf(ctx, "Run: suspend_stress_test -c %d", count)

	args := "{ nohup suspend_stress_test -c " + strconv.Itoa(count) + defaultWakeMin +
		defaultWakeMax + defaultSuspendMin + defaultSuspendMax +
		" --nopremature_wake_fatal --nocrc_fatal --nolate_wake_fatal --noerrors_fatal --nobug_fatal > " +
		suspendStressResults + " & } 2>/dev/null ; echo $!"
	pidSuspend, err := RunCmdWithStringOutput(ctx, dut, "bash", "-c", args)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute suspend_stress_test command")
	}
	testing.ContextLogf(ctx, "suspend pid is %s", pidSuspend)
	return pidSuspend, nil
}

// checkZeroFailure parses a suspend stress result line and if it is an error we care about,
// checks that we see 0 failures.
func checkZeroFailure(ctx context.Context, line string, errors []string) bool {
	strPieces := strings.Split(line, ":")
	for _, errMsg := range errors {
		if errMsg == strPieces[0] {
			val := strings.TrimSpace(strPieces[1])
			if val != "0" {
				testing.ContextLogf(ctx, "Found non-zero error: %s", line)
				return false
			}
		}
	}
	return true
}

// CheckSuspendStressResults parses the results file containing the SuspendStressTest
// output and checks for errors.
func CheckSuspendStressResults(ctx context.Context, dut *dut.DUT) error {
	const (
		prematureWakes    = "Premature wakes"
		suspendFailures   = "Suspend failures"
		firmwareLogErrors = "Firmware log errors"
		s0ixErrors        = "s0ix errors"
	)
	suspendErrors := []string{prematureWakes, suspendFailures, firmwareLogErrors, s0ixErrors}

	out, err := RunCmdWithStringOutputSilent(ctx, dut, "cat", suspendStressResults)
	if err != nil {
		return errors.Wrap(err, "failed to read suspend stress test results, device likely rebooted")
	}

	resultStart := strings.Index(out, "Finished")
	results := out[resultStart:]

	lines := strings.Split(results, "\n")
	for _, line := range lines {
		if !strings.Contains(line, ":") {
			continue
		}
		if !checkZeroFailure(ctx, line, suspendErrors) {
			res := strings.Replace(results, "\n", " ", -1)
			return errors.Errorf("failed: expect zero failures for %q, got %q", line, res)
		}
	}

	return nil

}

// CleanupSuspend removes the suspend stress test results file.
func CleanupSuspend(ctx context.Context, dut *dut.DUT) {
	if _, err := RunCmdWithOutput(ctx, dut, "rm", suspendStressResults); err != nil {
		testing.ContextLog(ctx, "Failed to cleanup suspend results: ", err)
	}
}
