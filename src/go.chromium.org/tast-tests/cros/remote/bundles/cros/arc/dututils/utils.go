// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dututils provides set of util functions used to work with remote dut.
package dututils

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"

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
	if path == "" {
		return nil, errors.New("failed to run cat due to invalid options")
	}
	output, err := d.Conn().CommandContext(ctx, "cat", path).Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// ChownRecRemote changes user and/or group ownership of a file at path recursively on the DUT.
func ChownRecRemote(ctx context.Context, d *dut.DUT, uid, gid, path string) error {
	if uid == "" || gid == "" || path == "" {
		return errors.New("failed to run chown due to invalid options")
	}
	if err := d.Conn().CommandContext(ctx, "chown", "-R", uid+":"+gid, path).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// ChmodRemote changes the permissions of file at path to new mode on the DUT.
func ChmodRemote(ctx context.Context, d *dut.DUT, mode, path string) error {
	if mode == "" || path == "" {
		return errors.New("failed to run chmod due to invalid options")
	}
	if err := d.Conn().CommandContext(ctx, "chmod", mode, path).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// FileRemote determines the type of a file as well as its data and returns to the output on the DUT.
func FileRemote(ctx context.Context, d *dut.DUT, path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("failed to run file due to invalid options")
	}
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
	if path == "" {
		return errors.New("failed to run mkdir due to invalid options")
	}
	if err := d.Conn().CommandContext(ctx, "mkdir", "-p", path).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// RemoveAllRemote recursively removes the directory at path on the DUT.
func RemoveAllRemote(ctx context.Context, d *dut.DUT, path string) error {
	if path == "" {
		return errors.New("path can not be empty")
	}
	if err := d.Conn().CommandContext(ctx, "rm", "-rf", path).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}

// MvRemote moves or renames files on the DUT.
func MvRemote(ctx context.Context, d *dut.DUT, sourcePath, destPath string) error {
	if sourcePath == "" || destPath == "" {
		return errors.New("failed to run mv due to invalid options")
	}
	if err := d.Conn().CommandContext(ctx, "mv", sourcePath, destPath).Run(testexec.DumpLogOnError); err != nil {
		return err
	}
	return nil
}
