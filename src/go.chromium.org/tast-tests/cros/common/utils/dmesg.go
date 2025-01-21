// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils defines utility functions common to local and remote tests.
package utils

import (
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// SaveDmesgToFile saves dmesg logs to file, useful when rebooting during test
func SaveDmesgToFile(ctx context.Context, outDir, testName string) error {
	dmesgOut, err := testexec.CommandContext(ctx, "sudo", "dmesg").CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "failed to dump dmesg logs")
	}
	fileName := strings.ReplaceAll(testName, " ", "_") + "_dmesg.txt"
	filePath := outDir + "/" + fileName
	if err := os.WriteFile(filePath, []byte(string(dmesgOut)), 0644); err != nil {
		return errors.Wrapf(err, "failed to save dmesg to %s", filePath)
	}
	return nil
}
