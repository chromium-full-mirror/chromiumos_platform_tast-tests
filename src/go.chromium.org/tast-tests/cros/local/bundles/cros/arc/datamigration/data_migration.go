// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datamigration

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// MountVaultWithArchivedHomeData mounts archived home data under the user's cryptohome.
func MountVaultWithArchivedHomeData(ctx context.Context, homeDataPath, username, password string) (cleanupFunc func(context.Context), retErr error) {
	// Unmount and mount vault for the user.
	if err := cryptohome.UnmountVault(ctx, username); err != nil {
		return func(context.Context) {}, err
	}
	if err := cryptohome.RemoveVault(ctx, username); err != nil {
		return func(context.Context) {}, err
	}
	if err := cryptohome.CreateVault(ctx, username, password); err != nil {
		return func(context.Context) {}, err
	}
	cleanupFunc = func(ctx context.Context) {
		cryptohome.UnmountVault(ctx, username)
		cryptohome.RemoveVault(ctx, username)
	}
	defer func() {
		if retErr != nil {
			cleanupFunc(ctx)
		}
	}()

	vaultPath, err := cryptohome.MountedVaultPath(ctx, username)
	if err != nil {
		return func(context.Context) {}, err
	}

	testing.ContextLogf(ctx, "Unarchiving home data %q under %q", homeDataPath, vaultPath)
	if err := testexec.CommandContext(
		ctx, "tar", "--xattrs", "--selinux", "-C", vaultPath, "-xjf", homeDataPath).Run(testexec.DumpLogOnError); err != nil {
		return func(context.Context) {}, errors.Wrap(err, "failed to unarchive home data under vault")
	}

	// Remove adb_temp_keys.xml from virtio-fs /data to avoid invalidating test adb key in T+
	// (b/289798262). For virtio-blk /data test cases running on T+, the file is already removed
	// before taking the snapshot.
	// For ARC R and earlier, this should be no-op.
	if err := os.RemoveAll(filepath.Join(vaultPath, "root/android-data/data/misc/adb/adb_temp_keys.xml")); err != nil {
		return func(context.Context) {}, errors.Wrap(err, "failed to remove adb_temp_keys.xml")
	}

	return cleanupFunc, nil
}
