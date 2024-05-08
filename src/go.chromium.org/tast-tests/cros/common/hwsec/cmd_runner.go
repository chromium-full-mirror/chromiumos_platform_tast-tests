// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

/*
This file declares the common interfaces of command runner.
*/

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// StderrLineFunc is the callback parameter type used in RunWithInteractiveStderr.
type StderrLineFunc func([]byte) error

// CmdRunner declares interface that runs command on DUT.
type CmdRunner interface {
	// Run runs the specified command and returns its standard output.
	Run(ctx context.Context, cmd string, args ...string) ([]byte, error)
	// RunWithCombinedOutput runs the specified command and returns its combined standard output and standard error.
	RunWithCombinedOutput(ctx context.Context, cmd string, args ...string) ([]byte, error)
	// RunWithInteractiveStderr runs the specified command, while triggering |onStderrLine| with each line in standard error
	// in real time. If |onStderrLine| returns an error, the command is aborted and the error is returned to the caller.
	// This can be used for communicating with interactive CLI tools.
	RunWithInteractiveStderr(ctx context.Context, onStderrLine StderrLineFunc, cmd string, args ...string) error
}

// CmdExitError is the error returned by CmdRunner when the command execution fail.
type CmdExitError struct {
	*errors.E
	ExitCode int
}
