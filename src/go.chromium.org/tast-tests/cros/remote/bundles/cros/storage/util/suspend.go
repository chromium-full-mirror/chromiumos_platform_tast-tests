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

// CheckSuspendStressResults parses the results file containing the SuspendStressTest
// output and checks for errors.
func CheckSuspendStressResults(ctx context.Context, dut *dut.DUT) error {
	const (
		zeroPrematureWakes    = "Premature wakes: 0"
		zeroSuspendFailures   = "Suspend failures: 0"
		zeroFirmwareLogErrors = "Firmware log errors: 0"
		zeroS0ixErrors        = "s0ix errors: 0"
	)
	zeroSuspendErrors := []string{zeroPrematureWakes, zeroSuspendFailures, zeroFirmwareLogErrors, zeroS0ixErrors}

	out, err := RunCmdWithStringOutputSilent(ctx, dut, "cat", suspendStressResults)
	if err != nil {
		return errors.Wrap(err, "failed to read suspend stress test results, device likely rebooted")
	}

	for _, errMsg := range zeroSuspendErrors {
		if !strings.Contains(out, errMsg) {
			out = strings.Replace(out, "\n", " ", -1)
			resultStart := strings.Index(out, "Finished")
			results := out[resultStart:]
			return errors.Errorf("failed: expect zero failures for %q, got %q", errMsg, results)
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
