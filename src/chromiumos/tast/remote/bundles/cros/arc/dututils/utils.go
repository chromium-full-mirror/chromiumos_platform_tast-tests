// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dututils provides set of util functions used to work with remote dut.
package dututils

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// AndroidShellRemote runs a command with "android-sh" and returns the output on the DUT. This
// command should only be called after Android is up and running successfully.
func AndroidShellRemote(ctx context.Context, d *dut.DUT, command string) ([]byte, error) {
	if !strings.HasPrefix(command, "/") {
		return nil, errors.Errorf("failed to execute %q, please specify an absolute command path instead", command)
	}
	output, err := d.Conn().CommandContext(ctx, "android-sh", "-c", command).Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// CatRemote reads data from the path and concatenates content to the output on the DUT.
func CatRemote(ctx context.Context, d *dut.DUT, path string) ([]byte, error) {
	output, err := d.Conn().CommandContext(ctx, "cat", path).Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// FileRemote determines the type of a file as well as its data and returns to the output on the DUT.
func FileRemote(ctx context.Context, d *dut.DUT, path string) ([]byte, error) {
	output, err := d.Conn().CommandContext(ctx, "file", path).Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// LsCPURemote gathers CPU architecture information and returns the output on the DUT.
func LsCPURemote(ctx context.Context, d *dut.DUT) ([]byte, error) {
	output, err := d.Conn().CommandContext(ctx, "lscpu").Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

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
