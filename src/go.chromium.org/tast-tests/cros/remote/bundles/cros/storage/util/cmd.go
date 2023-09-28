// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains utility functions for storage tests.
package util

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/testing"
)

// RunCmdWithOutput runs a command on the DUT and proiduces its output or error.
func RunCmdWithOutput(ctx context.Context, dut *dut.DUT, bin string, args ...string) ([]byte, error) {
	cmd := dut.Conn().CommandContext(ctx, bin, args...)
	testing.ContextLog(ctx, "Running command: ", cmd)
	return cmd.Output(testexec.DumpLogOnError)
}

// RunCmdWithOutputSilent runs a command on the DUT and produces its output or error without logging.
func RunCmdWithOutputSilent(ctx context.Context, dut *dut.DUT, bin string, args ...string) ([]byte, error) {
	cmd := dut.Conn().CommandContext(ctx, bin, args...)
	return cmd.Output(testexec.DumpLogOnError)
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
