// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

/*
This file implements command runner for local tests.
*/

import (
	"bufio"
	"context"
	"os/exec"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// CmdRunnerLocal implements CmdRunner for local test.
type CmdRunnerLocal struct {
	printLog bool
}

// NewCmdRunner creates a new command runner for local test.
func NewCmdRunner() *CmdRunnerLocal {
	return &CmdRunnerLocal{printLog: true}
}

// NewLoglessCmdRunner creates a new command runner for local test, which wouldn't print logs.
func NewLoglessCmdRunner() *CmdRunnerLocal {
	return &CmdRunnerLocal{printLog: false}
}

// Run implements hwsec.CmdRunner.Run.
func (r *CmdRunnerLocal) Run(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	if r.printLog {
		testing.ContextLogf(ctx, "Running: %s", shutil.EscapeSlice(append([]string{cmd}, args...)))
	}
	result, err := testexec.CommandContext(ctx, cmd, args...).Output()
	err = checkExitError(cmd, err)
	return result, err
}

// RunWithCombinedOutput implements hwsec.CmdRunner.RunWithCombinedOutput.
func (r *CmdRunnerLocal) RunWithCombinedOutput(ctx context.Context, cmd string, args ...string) ([]byte, error) {
	if r.printLog {
		testing.ContextLogf(ctx, "Running: %s", shutil.EscapeSlice(append([]string{cmd}, args...)))
	}
	result, err := testexec.CommandContext(ctx, cmd, args...).CombinedOutput()
	err = checkExitError(cmd, err)
	return result, err
}

// RunWithInteractiveStderr implements hwsec.CmdRunner.RunWithInteractiveStderr.
func (r *CmdRunnerLocal) RunWithInteractiveStderr(ctx context.Context, onStderrLine hwsec.StderrLineFunc, cmd string, args ...string) error {
	if r.printLog {
		testing.ContextLogf(ctx, "Running: %s", shutil.EscapeSlice(append([]string{cmd}, args...)))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	command := testexec.CommandContext(ctx, cmd, args...)
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "failed to get stderr pipe")
	}
	if err := command.Start(); err != nil {
		return errors.Wrap(err, "failed to start command")
	}

	sc := bufio.NewScanner(stderrPipe)
	sc.Split(bufio.ScanLines)
	for sc.Scan() {
		if err := onStderrLine(sc.Bytes()); err != nil {
			cancel()
			return err
		}
	}

	err = command.Wait()
	err = checkExitError(cmd, err)
	return err
}

func checkExitError(cmd string, err error) error {
	if e, ok := err.(*exec.ExitError); ok && e.Exited() {
		err = &hwsec.CmdExitError{
			E:        errors.Wrapf(err, "failed %q command with error code %d", cmd, e.ExitCode()),
			ExitCode: e.ExitCode(),
		}
	}
	return err
}
