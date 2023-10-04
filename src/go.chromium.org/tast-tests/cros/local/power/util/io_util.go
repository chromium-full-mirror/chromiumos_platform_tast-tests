// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// getRootDev returns root device of the DUT.
func getRootDev(ctx context.Context) (string, error) {
	// Get the root device name. Example: /dev/nvme0n1
	cmd := testexec.CommandContext(ctx, "rootdev", "-s", "-d")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run rootdev command")
	}
	outStr := strings.TrimSpace(string(out))

	// Convert /dev/nvme0n1 to nvme0n1
	return filepath.Base(outStr), nil
}

// getIOTps return I/O transfer in the last second of given block device.
func getIOTps(ctx context.Context, device string) (float64, error) {
	iostatCmd := "iostat -dy 1 1 " + device + " | grep " + device + " | awk '{print $2}'"
	cmd := testexec.CommandContext(ctx, "sh", "-c", iostatCmd)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "failed to run iostat command")
	}
	outStr := strings.TrimSpace(string(out))
	ret, err := strconv.ParseFloat(outStr, 64)
	if err != nil {
		return 0, errors.Wrap(err, "Fail to convert iostat output to float64")
	}
	return ret, nil
}

// pollTpsToTarget poll I/O TPS until drop to target.
func pollTpsToTarget(ctx context.Context, device string, target float64) error {
	testing.ContextLogf(ctx, "Wait up to 5 minutes for I/O TPS to not exceed %f", target)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		tps, err := getIOTps(ctx, device)
		if err != nil {
			return testing.PollBreak(err)
		}
		if tps <= target {
			return nil
		}
		return errors.Errorf("TPS is too high: %f", tps)
	}, &testing.PollOptions{Timeout: 5 * time.Minute}); err != nil {
		return errors.Wrapf(err, "Target: %f", target)
	}
	return nil
}

// WaitForIOCooldown poll iostat command to wait for I/O activity.
func WaitForIOCooldown(ctx context.Context) error {
	rootdev, err := getRootDev(ctx)
	if err != nil {
		return err
	}

	timeBefore := time.Now()

	// Try for 5 minutes for I/O to drop to 0 TPS (transfer per second).
	// If not met, then another 5 minutes for 5 TPS.
	for target := 0; target <= 5; target += 5 {
		err = pollTpsToTarget(ctx, rootdev, float64(target))
		if err == nil {
			break
		}
	}

	if err != nil {
		return err
	}

	timeAfter := time.Now()
	duration := timeAfter.Sub(timeBefore)
	testing.ContextLogf(ctx, "I/O is cooled down (took %f seconds)", duration.Seconds())

	return nil
}
