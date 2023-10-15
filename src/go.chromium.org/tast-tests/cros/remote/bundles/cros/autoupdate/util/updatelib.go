// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/updateutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

const (
	cleanupTimeoutN2M    = 30 * time.Second
	initTimeoutN2M       = 3 * time.Minute
	preUpdateTimeoutN2M  = 3 * time.Minute
	postUpdateTimeoutN2M = 3 * time.Minute
	// TotalTestTime is the maximum time the test expected to take.
	TotalTestTime = cleanupTimeoutN2M + initTimeoutN2M + preUpdateTimeoutN2M + postUpdateTimeoutN2M + updateutil.UpdateTimeout
)

// Operations contains operations performed at various points of update sequence.
// Each operation has timeout 3 minutes, except cleanup, which is 30 seconds.
type Operations struct {
	PreUpdate    func(ctx context.Context) error
	PostUpdate   func(ctx context.Context) error
	PostRollback func(ctx context.Context) error
	CleanUp      func(ctx context.Context)
}

func getModel(ctx context.Context, dut *dut.DUT) (string, error) {
	b, err := dut.Conn().CommandContext(ctx, "cros_config", "/", "name").Output()
	if err != nil {
		return "", errors.Wrap(err, "cros_config failed")
	}
	if len(b) == 0 {
		return "", errors.New("cros_config returned empty model")
	}
	return strings.TrimSpace(string(b)), nil
}

// NToMTest drives autoupdate and calls to client code providing callbacks.
// paygen parameter specifies the filtered paygen the test wants to apply,
// where the board type and milestone will be filtered in this function.
// deltaM parameter specifies amount of milestones to rollback.
func NToMTest(ctx context.Context, dut *dut.DUT, outDir string, rpcHint *testing.RPCHint, ops *Operations, paygen *updateutil.Paygen, deltaM int) error {
	// Reserve time for deferred calls.
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTimeoutN2M)
	defer cancel()

	if ops.CleanUp != nil {
		defer ops.CleanUp(ctxForCleanUp)
	}

	// Limit the timeout for init steps.
	initCtx, cancel := context.WithTimeout(ctx, initTimeoutN2M)
	defer cancel()

	if ops.PreUpdate != nil {
		testing.ContextLog(ctx, "Running PreUpdate")
		if err := ops.PreUpdate(initCtx); err != nil {
			return errors.Wrap(err, "failed to run the PreUpdate operation")
		}
	}

	// Limit the timeout for the preparation steps.
	preCtx, cancel := context.WithTimeout(ctx, preUpdateTimeoutN2M)
	defer cancel()

	lsbContent := map[string]string{
		lsbrelease.Board:     "",
		lsbrelease.Version:   "",
		lsbrelease.Milestone: "",
	}

	err := updateutil.FillFromLSBRelease(preCtx, dut, rpcHint, lsbContent)
	if err != nil {
		return errors.Wrap(err, "failed to get all the required information from lsb-release")
	}

	board := lsbContent[lsbrelease.Board]
	originalVersion := lsbContent[lsbrelease.Version]
	model, err := getModel(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get model of device")
	}

	milestoneN, err := strconv.Atoi(lsbContent[lsbrelease.Milestone])
	if err != nil {
		return errors.Wrapf(err, "failed to convert milestone to integer %s", lsbContent[lsbrelease.Milestone])
	}
	milestoneM := milestoneN - deltaM // Target milestone.

	filtered := paygen.FilterBoard(board).FilterMilestone(milestoneM)
	latest, err := filtered.FindLatest()
	if err != nil {
		return errors.Wrapf(err, "failed to find the latest release for milestone %d, board %s, and model %s", milestoneM, board, model)
	}
	rollbackVersion := latest.ChromeOSVersion

	builderPath := fmt.Sprintf("%s-release/R%d-%s", board, milestoneM, rollbackVersion)

	// Update the DUT.
	testing.ContextLogf(ctx, "Starting update from %s to %s", originalVersion, rollbackVersion)
	if err := updateutil.UpdateFromGS(ctx, dut, outDir, rpcHint, builderPath); err != nil {
		return errors.Wrapf(err, "failed to update DUT to image for %q from GS", builderPath)
	}

	// Limit the timeout for the verification steps.
	postCtx, cancel := context.WithTimeout(ctx, postUpdateTimeoutN2M)
	defer cancel()

	// Reboot the DUT.
	testing.ContextLog(ctx, "Rebooting the DUT after the update")
	if err := dut.Reboot(postCtx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT after update")
	}

	// Check the image version.
	version, err := updateutil.ImageVersion(postCtx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to read DUT image version after the update")
	}
	testing.ContextLogf(ctx, "The DUT image version after the update is %s", version)
	if version != rollbackVersion {
		if version == originalVersion {
			// Rollback is not needed here, the test execution can stop.
			return errors.New("the image version did not change after the update")
		}
		return errors.Wrapf(err, "failed to update the image, image version after the update is incorrect; got %s, want %s", version, rollbackVersion)
	}

	if ops.PostUpdate != nil {
		testing.ContextLog(ctx, "Running PostUpdate")
		if err := ops.PostUpdate(postCtx); err != nil {
			return errors.Wrap(err, "failed to run the PostUpdate operation")
		}
	}

	// Restore original image version with rollback.
	testing.ContextLog(ctx, "Restoring the original device image")
	if out, err := dut.Conn().CommandContext(postCtx, "update_engine_client", "--rollback", "--nopowerwash", "--follow").CombinedOutput(); err != nil {
		return errors.Wrapf(err, "failed to rollback the DUT, command output: %q", string(out))
	}

	// Reboot the DUT.
	testing.ContextLog(ctx, "Rebooting the DUT after the rollback")
	if err := dut.Reboot(postCtx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT after rollback")
	}

	// Check the image version.
	version, err = updateutil.ImageVersion(postCtx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to read DUT image version after the update")
	}
	testing.ContextLogf(ctx, "The DUT image version after the rollback is %s", version)
	if version != originalVersion {
		return errors.Errorf("image version is not the original after the restoration; got %s, want %s", version, originalVersion)
	}

	if ops.PostRollback != nil {
		testing.ContextLog(ctx, "Running PostRollback")
		if err := ops.PostRollback(postCtx); err != nil {
			return errors.Wrap(err, "failed to run the PostRollback operation")
		}
	}

	return nil
}
