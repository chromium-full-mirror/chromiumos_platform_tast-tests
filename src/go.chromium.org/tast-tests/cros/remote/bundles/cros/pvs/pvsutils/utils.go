// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvsutils

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

type testState interface {
	Logf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// RunAsChronos runs the given command as the chronos user on the given dut.
func RunAsChronos(ctx context.Context, dut *ssh.Conn, cmd string) (string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	return runAsRoot(ctx, wrappedCmd)
}

// RunPVSSubcommand runs the given pvs subcommand against the given container on the given dut.
func RunPVSSubcommand(ctx context.Context, dut *ssh.Conn, containerID, subcommand string) (string, error) {
	pvsCommand := fmt.Sprintf(
		`docker exec %q /usr/bin/gosu pvs pvs %v`,
		containerID,
		subcommand,
	)
	return RunAsChronos(ctx, dut, pvsCommand)
}

func runAsRoot(ctx context.Context, cmd *ssh.Cmd) (string, error) {
	testing.ContextLogf(ctx, "Running command: `%v`", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	testing.ContextLog(ctx, "Output from command: ", string(out))
	return string(out), err
}

func removeAsRoot(ctx context.Context, dut *ssh.Conn, path string) (string, error) {
	cmd := dut.CommandContext(ctx, "sudo", "rm", "-rf", path)
	return runAsRoot(ctx, cmd)
}

func runAsChronosWithStdin(ctx context.Context, dut *ssh.Conn, cmd, stdin string) (string, error) {
	wrappedCmd := dut.CommandContext(ctx, "sudo", "--login", "-u", "chronos", "bash", "-c", cmd)
	wrappedCmd.Stdin = strings.NewReader(stdin)
	return runAsRoot(ctx, wrappedCmd)
}

func writeToFileAsChronos(ctx context.Context, dut *ssh.Conn, content, path string) (string, error) {
	writeToFile := fmt.Sprintf(`cat > %v`, path)
	return runAsChronosWithStdin(ctx, dut, writeToFile, content)
}
