// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DirectUpdate updates the DUT when rollback is not required.
// TODO(b/323024317): Ensure method works for ti50 as well.
func (b *DUTControlAndreiboard) DirectUpdate(ctx context.Context, i *common.CrOSImage, imagePath string) error {
	_, imageVer, _, rwb, err := b.GSCToolBinVersion(ctx, imagePath)
	if err != nil {
		return errors.Wrap(err, "parse bin version")
	}

	if imageVer.String() != rwb.String() {
		return errors.Errorf("imageVer a and b differ: %s != %s", imageVer, rwb)
	}

	if err := b.UpdateOnce(ctx, i, imagePath, imageVer); err != nil {
		return errors.Wrap(err, "update slot 1")
	}

	return b.FlashDuplicateImage(ctx, i, imagePath, imageVer)
}

// Rollback performs rollback by flashing to debug image, then running rollback command on the debug image.
func (b *DUTControlAndreiboard) Rollback(ctx context.Context, i *common.CrOSImage, imageVer GSCVersion) error {
	if err := i.WaitUntilBooted(ctx); err != nil {
		return errors.Wrap(err, "wait for debug image to boot")
	}

	if _, err := i.Command(ctx, "rollback"); err != nil {
		return errors.Wrap(err, "rollback")
	}

	if err := b.GSCToolWaitUntilReady(ctx); err != nil {
		return errors.Wrap(err, "wait until gsc ready after rollback")
	}

	matched, err := i.CheckRunningVersion(ctx, imageVer.String(), false, false)
	if err != nil {
		return errors.Wrap(err, "failed to get current fwver after rollback")
	}
	if !matched {
		return errors.Wrap(err, "image not running after rollback")
	}

	return nil
}

// RollbackUpdate performs rollback update by flashing to debug image, inactive with image, then running rollback command on the debug image.
// TODO(b/323024317): Ensure method works for ti50 as well.
func (b *DUTControlAndreiboard) RollbackUpdate(ctx context.Context, i *common.CrOSImage, imagePath, debugImage string) error {
	_, imageVer, _, _, err := b.GSCToolBinVersion(ctx, imagePath)
	if err != nil {
		return err
	}

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	if err != nil {
		return err
	}

	// Update to the debug image if it's not running or if there's an error getting the version.
	matched, err := i.CheckRunningVersion(ctx, imageVer.String(), false, true)
	if err != nil || !matched {
		if err != nil {
			testing.ContextLogf(ctx, "Failed to get version: %s", err)
		}
		if err = b.UpdateOnce(ctx, i, debugImage, debugVer); err != nil {
			return errors.Wrap(err, "failed to update to debug image")
		}
	}

	if err = b.GSCToolUpdate(ctx, i, imagePath); err != nil {
		return errors.Wrap(err, "failed to update inactive 1 to image")
	}

	if err = b.GSCToolWaitUntilReady(ctx); err != nil {
		return errors.Wrap(err, "failed wait until gsc ready after image update")
	}

	if err = b.Rollback(ctx, i, imageVer); err != nil {
		return errors.Wrapf(err, "failed to rollback to %s", imageVer.String())
	}
	return b.FlashDuplicateImage(ctx, i, imagePath, imageVer)
}

// FlashDuplicateImage flashes the inactive slot and verifies RW_A and RW_B are
// running the same version after update.
func (b *DUTControlAndreiboard) FlashDuplicateImage(ctx context.Context, i *common.CrOSImage, imagePath string, imageVer GSCVersion) error {
	testing.ContextLog(ctx, "Sleeping for 61 seconds to avoid update too soon error")
	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, 61*time.Second)

	if err := b.UpdateOnce(ctx, i, imagePath, imageVer); err != nil {
		return errors.Wrap(err, "update inactive 2 to image")
	}

	if err := b.CheckEqualConsoleVersions(ctx, i, imageVer); err != nil {
		return errors.Wrap(err, "version equality after updating both slots")
	}
	return nil
}

// UpdateOnce updates and checks the version using gsctool.
func (b *DUTControlAndreiboard) UpdateOnce(ctx context.Context, i *common.CrOSImage, imagePath string, wantVer GSCVersion) error {
	if err := b.GSCToolUpdate(ctx, i, imagePath); err != nil {
		return errors.Wrap(err, "image update")
	}

	if err := b.GSCToolWaitUntilReady(ctx); err != nil {
		return errors.Wrap(err, "wait until gsc ready after image update")
	}

	_, ver, err := b.GSCToolCurrentFwVersion(ctx)
	if err != nil {
		return errors.Wrap(err, "get fwver after image update")
	}

	if ver != wantVer {
		return errors.Errorf("version check after image update, want %v, got %v", wantVer, ver)
	}

	return nil
}

// CheckEqualConsoleVersions ensures that both slots have the same version as reported
// with the "version" command.  GSC console must be closed before call.
func (b *DUTControlAndreiboard) CheckEqualConsoleVersions(ctx context.Context, i *common.CrOSImage, imageVer GSCVersion) error {
	if err := i.WaitUntilBooted(ctx); err != nil {
		return errors.Wrap(err, "wait image to boot")
	}

	matched, err := i.CheckRunningVersion(ctx, imageVer.String(), false, true)
	if err != nil {
		return err
	}
	if !matched {
		return errors.Errorf("GSC is not runnning %s in both slots", imageVer)
	}
	return nil
}
