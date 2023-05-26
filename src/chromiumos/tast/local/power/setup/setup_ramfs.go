// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func setUpRamfs(ctx context.Context) (CleanupCallback, error) {

	if err := testexec.CommandContext(ctx, "mkdir", "-p", "/tmp/ramdisk").Run(); err != nil {
		return nil, errors.Wrap(err, "can't create directory to mount ramfs")
	}

	if err := testexec.CommandContext(ctx, "mount", "-t", "ramfs", "-o", "context=u:object_r:tmpfs:s0", "ramfs", "/tmp/ramdisk").Run(); err != nil {
		testing.ContextLog(ctx, "Can't mount ramfs with context=u:object_r:tmpfs:s0, trying plain mount")
		if err := testexec.CommandContext(ctx, "mount", "-t", "ramfs", "ramfs", "/tmp/ramdisk").Run(); err != nil {
			return nil, errors.Wrap(err, "can't plain mount ramfs")
		}
	}
	return tearDownRamfs, nil
}

func tearDownRamfs(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "umount", "/tmp/ramdisk").Run(); err != nil {
		return errors.Wrap(err, "can't umount ramfs")
	}
	return nil
}
