// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// XattrName is the extended attribute name on ~/MyFiles/Downloads folder.
	XattrName = "user.BindMountMigration"
	// TestFile is the file name used to test removal of bind mount.
	TestFile = "test_1KB.txt"
)

// DownloadsExistAsBindMount checks if "Downloads" exists as a bind mount or not.
func DownloadsExistAsBindMount(ctx context.Context) (bool, error) {
	result, err := testexec.CommandContext(ctx, "findmnt").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to run findmnt command")
	}
	return strings.Contains(string(result), "Downloads"), nil
}

// VerifyFileContent reads the `filePath` and compare its content with the
// `wantedContent`, throws error if they don't match.
func VerifyFileContent(filePath, wantedContent string) error {
	gotContent, err := os.ReadFile(filePath)
	if err != nil {
		return errors.Wrapf(err, "failed to read test file %q", filePath)
	}
	if !strings.EqualFold(string(gotContent), wantedContent) {
		return errors.Errorf("file content don't match: got %q want %q", gotContent, wantedContent)
	}
	return nil
}

// RestartCryptohomed restart cryptohomed with download bind mount enabled or not.
func RestartCryptohomed(ctx context.Context, enableDownloadsBindMount bool) error {
	testing.ContextLogf(ctx, "Restarting cryptohomed with downloads_bind_mount=%t", enableDownloadsBindMount)
	if enableDownloadsBindMount {
		return upstart.RestartJob(ctx, "cryptohomed")
	}
	return upstart.RestartJob(ctx, "cryptohomed", upstart.WithArg("CRYPTOHOMED_ARGS", "--no_downloads_bind_mount"))
}
