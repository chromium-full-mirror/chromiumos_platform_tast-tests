// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// SetWP enables/disables software write protection.
//
// Returns program output, and error on failure.
func (i *Instance) SetWP(ctx context.Context, enable bool) ([]byte, error) {
	cmdArgs := append(i.futilityCmdArgs(), "flash")
	if enable {
		cmdArgs = append(cmdArgs, "--wp-enable")
	} else {
		cmdArgs = append(cmdArgs, "--wp-disable")
	}
	cmdArgs = i.appendFlashArgs(cmdArgs)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrap(err, "failed to set the write protection")
	}

	return fullOut, nil
}
