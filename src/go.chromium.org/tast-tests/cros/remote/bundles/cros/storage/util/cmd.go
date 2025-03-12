// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains utility functions for storage tests.
package util

import (
	"context"
	"strings"

	"golang.org/x/crypto/ssh"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const maxRetry = 3

// RunCmdWithOutput runs a command on the DUT and proiduces its output or error.
func RunCmdWithOutput(ctx context.Context, dut *dut.DUT, bin string, args ...string) ([]byte, error) {
	testing.ContextLogf(ctx, "Running command: %q", append([]string{bin}, args...))
	return RunCmdWithOutputSilent(ctx, dut, bin, args...)
}

// RunCmdWithOutputSilent runs a command on the DUT and produces its output or error without logging.
func RunCmdWithOutputSilent(ctx context.Context, dut *dut.DUT, bin string, args ...string) ([]byte, error) {
	for i := 0; i < maxRetry; i++ {
		cmd := dut.Conn().CommandContext(ctx, bin, args...)
		if out, err := cmd.Output(testexec.DumpLogOnError); err == nil || !errors.Is(err, &ssh.ExitMissingError{}) {
			return out, err
		}
		// SSH command may fail and return ExitMissingError when the SSH session
		// disconnected due to unstable network environment.
		// Try to reconnect to DUT and retry the command.
		testing.ContextLogf(ctx, "SSH session disconnected. Reconnect and retry the command: %q", append([]string{bin}, args...))
		if err := dut.Connect(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to reconnect to the DUT")
		}
	}
	return nil, errors.Wrapf(&ssh.ExitMissingError{}, "failed to run command %q after retrying for %d times", append([]string{bin}, args...), maxRetry)
}

// RunCmdWithStringOutput is a wrapper to RunCmdWithOutput which converts the
// output to a string.
func RunCmdWithStringOutput(ctx context.Context, dut *dut.DUT, bin string, args ...string) (string, error) {
	out, err := RunCmdWithOutput(ctx, dut, bin, args...)
	return strings.TrimSpace(string(out)), err
}

// RunCmdWithStringOutputSilent is a wrapper to RunCmdWithOutput which converts the
// output to a string without logging.
func RunCmdWithStringOutputSilent(ctx context.Context, dut *dut.DUT, bin string, args ...string) (string, error) {
	out, err := RunCmdWithOutputSilent(ctx, dut, bin, args...)
	return strings.TrimSpace(string(out)), err
}
