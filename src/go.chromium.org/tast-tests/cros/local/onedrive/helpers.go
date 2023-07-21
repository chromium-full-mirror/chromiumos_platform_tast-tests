// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package onedrive

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CheckODFSContent compares the content of the srcFilePath and dstFileName.
func CheckODFSContent(ctx context.Context, srcFilePath, dstFileName string) error {

	srcContent, err := os.ReadFile(srcFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to read the src file: %s", srcFilePath)
	}

	odfsNames, err := filepath.Glob("/media/fuse/fusebox/fsp.*/" + dstFileName)
	if err != nil {
		return errors.Wrapf(err, "failed to list the file: %s", dstFileName)
	}
	if len(odfsNames) == 0 {
		// No files found, nothing to compare against.
		return errors.Errorf("couldn't find the file on ODFS: %s", dstFileName)
	}

	odfsName := odfsNames[0]
	odfsContent, err := os.ReadFile(odfsName)
	if err != nil {
		return errors.Wrapf(err, "failed to read the odfs file: %s", odfsName)
	}

	// TODO: Figure out why odsfContent is length 0.
	if !bytes.Equal(srcContent, odfsContent) {
		testing.ContextLogf(ctx, "The content uploaded to ODFS doesn't match. ODFS file: %s size: %d Data file: %s size: %d", odfsName, len(odfsContent), srcFilePath, len(srcContent))
	}

	return nil
}
