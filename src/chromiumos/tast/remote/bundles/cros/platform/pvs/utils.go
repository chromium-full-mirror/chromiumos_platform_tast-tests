// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

type testState interface {
	Logf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// RunAsChronos runs the given command as the chronos user on the given dut
func RunAsChronos(ctx context.Context, dut *ssh.Conn, cmd string) (string, error) {
	wrappedCmd := dut.CommandContext(ctx, "runuser", "-l", "chronos", "-c", cmd)
	return runAsRoot(ctx, wrappedCmd)
}

func runAsRoot(ctx context.Context, cmd *ssh.Cmd) (string, error) {
	testing.ContextLogf(ctx, "Running command: `%v`", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	testing.ContextLog(ctx, "Output from command: ", string(out))
	return string(out), err
}

func removeDirAsRoot(ctx context.Context, dut *ssh.Conn, dir string) (string, error) {
	cmd := dut.CommandContext(ctx, "sudo", "rm", "-rf", dir)
	return runAsRoot(ctx, cmd)
}
