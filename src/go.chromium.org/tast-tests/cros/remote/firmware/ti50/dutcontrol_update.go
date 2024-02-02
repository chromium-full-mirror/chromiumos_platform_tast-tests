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
func (b *DUTControlAndreiboard) DirectUpdate(ctx context.Context, imagePath string) error {
	_, imageVer, _, rwb, err := b.GSCToolBinVersion(ctx, imagePath)
	if err != nil {
		return errors.Wrap(err, "parse bin version")
	}

	if imageVer.String() != rwb.String() {
		return errors.Errorf("imageVer a and b differ: %s != %s", imageVer, rwb)
	}

	if err := b.UpdateOnce(ctx, imagePath, imageVer); err != nil {
		return errors.Wrap(err, "update slot 1")
	}

	testing.ContextLog(ctx, "Sleeping for 61 seconds to avoid update too soon error")
	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, 61*time.Second)

	if err := b.UpdateOnce(ctx, imagePath, imageVer); err != nil {
		return errors.Wrap(err, "update slot 2")
	}

	if err := b.CheckEqualConsoleVersions(ctx); err != nil {
		return errors.Wrap(err, "console versions are equal")
	}

	return nil
}

// Rollback performs rollback by flashing to debug image, then running rollback command on the debug image.
// The GSC UART must be closed before calling this method since it opens it to issue commands to the board.
func (b *DUTControlAndreiboard) Rollback(ctx context.Context) error {
	gscConsole := b.PhysicalUart(common.UartConsole)

	i, err := common.OpenCrOSImage(ctx, gscConsole)
	if err != nil {
		return errors.Wrap(err, "open cros image")
	}
	defer i.Close(ctx)

	if err = i.WaitUntilBooted(ctx); err != nil {
		return errors.Wrap(err, "wait for debug image to boot")
	}

	consoleVer, err := i.GetVersionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get version from gsc console")
	}

	if consoleVer.RwA.Active == consoleVer.RwB.Active {
		return errors.Errorf("slot A and B are similarly active: %v", consoleVer.RwA)
	}

	var inactiveVer common.RwInfo
	if !consoleVer.RwA.Active {
		inactiveVer = consoleVer.RwA
	} else {
		inactiveVer = consoleVer.RwB
	}

	if _, err = i.Command(ctx, "rollback"); err != nil {
		return errors.Wrap(err, "rollback")
	}

	if err = b.GSCToolWaitUntilReady(ctx); err != nil {
		return errors.Wrap(err, "wait until gsc ready after rollback")
	}

	_, rw, err := b.GSCToolCurrentFwVersion(ctx)
	if err != nil {
		return errors.Wrap(err, "get current fwver after rollback")
	}

	if rw.String() != inactiveVer.Version {
		return errors.Errorf("Version after rollback not correct, got %s, want %s", rw.String(), inactiveVer.Version)
	}

	return nil
}

// RollbackUpdate performs rollback update by flashing to debug image, inactive with image, then running rollback command on the debug image.
// The GSC UART must be closed before calling this method since it opens it to issue commands to the board.
// TODO(b/323024317): Ensure method works for ti50 as well.
func (b *DUTControlAndreiboard) RollbackUpdate(ctx context.Context, imagePath, debugImage string) error {
	_, imageVer, _, _, err := b.GSCToolBinVersion(ctx, imagePath)
	if err != nil {
		return errors.Wrap(err, "parse bin version")
	}

	_, debugVer, _, _, err := b.GSCToolBinVersion(ctx, debugImage)
	if err != nil {
		return err
	}

	if err = b.UpdateOnce(ctx, debugImage, debugVer); err != nil {
		return errors.Wrap(err, "update to debug image")
	}

	if err = b.GSCToolUpdate(ctx, imagePath); err != nil {
		return errors.Wrap(err, "update inactive 1 to image")
	}

	if err = b.Rollback(ctx); err != nil {
		return errors.Wrap(err, "rollback")
	}

	testing.ContextLog(ctx, "Sleeping for 61 seconds to avoid update too soon error")
	// GoBigSleepLint sleeping for known required period of time.
	testing.Sleep(ctx, 61*time.Second)

	if err = b.UpdateOnce(ctx, imagePath, imageVer); err != nil {
		return errors.Wrap(err, "update inactive 2 to image")
	}

	if err = b.CheckEqualConsoleVersions(ctx); err != nil {
		return errors.Wrap(err, "version equality after updating both slots")
	}
	return nil
}

// UpdateOnce updates and checks the version using gsctool.
func (b *DUTControlAndreiboard) UpdateOnce(ctx context.Context, imagePath string, wantVer GSCVersion) error {
	if err := b.GSCToolUpdate(ctx, imagePath); err != nil {
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
func (b *DUTControlAndreiboard) CheckEqualConsoleVersions(ctx context.Context) error {
	gscConsole := b.PhysicalUart(common.UartConsole)

	i, err := common.OpenCrOSImage(ctx, gscConsole)
	if err != nil {
		return errors.Wrap(err, "open cros image")
	}
	defer i.Close(ctx)

	if err = i.WaitUntilBooted(ctx); err != nil {
		return errors.Wrap(err, "wait image to boot")
	}

	consoleVer, err := i.GetVersionInfo(ctx)
	if err != nil {
		return errors.Wrap(err, "get version from gsc console")
	}

	if consoleVer.RwA.Version != consoleVer.RwB.Version || consoleVer.RwA.Branch != consoleVer.RwB.Branch {
		return errors.Errorf("slot A and B version differ, A: %v, B: %v", consoleVer.RwA, consoleVer.RwB)
	}

	return nil
}
