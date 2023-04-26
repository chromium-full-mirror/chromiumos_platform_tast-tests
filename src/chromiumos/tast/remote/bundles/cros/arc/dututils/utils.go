// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dututils provides set of util functions used to work with remote dut.
package dututils

import (
	"context"

	"go.chromium.org/tast/core/dut"
)

// MkdirRemote creates directory at path including parent directories as needed on the DUT.
func MkdirRemote(ctx context.Context, d *dut.DUT, path string) error {
	if err := d.Conn().CommandContext(ctx, "mkdir", "-p", path).Run(); err != nil {
		return err
	}
	return nil
}

// RemoveAllRemote recursively removes the directory at path on the DUT.
func RemoveAllRemote(ctx context.Context, d *dut.DUT, path string) error {
	if err := d.Conn().CommandContext(ctx, "rm", "-rf", path).Run(); err != nil {
		return err
	}
	return nil
}
