// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// RunCommandWithRetry runs a command and retries up to `attempts` times with
// `interval` duration apart until success. This function returns error if all
// attempts at running the command failed.
func RunCommandWithRetry(ctx context.Context, attempts int, interval time.Duration, errMsg, cmd string, args ...string) error {
	cmdAction := func(cctx context.Context) error {
		if err := testexec.CommandContext(cctx, cmd, args...).Run(); err != nil {
			return errors.Wrap(err, errMsg)
		}
		return nil
	}

	if err := action.Retry(attempts, cmdAction, interval)(ctx); err != nil {
		return err
	}

	return nil
}
