// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package onedrive

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CheckODFSContent compares the content of the srcFilePath and dstFileName.
func CheckODFSContent(ctx context.Context, srcFilePath, dstFileName string) error {

	checkerFunc := func(ctx context.Context) error {
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

		if !bytes.Equal(srcContent, odfsContent) {
			testing.ContextLogf(ctx, "The content uploaded (%q) to ODFS doesn't match the remote file (%s) size want: %d got: %d", srcFilePath, odfsName, len(srcContent), len(odfsContent))
		}

		return nil
	}

	return action.RetryWithExponentialBackoff(10, checkerFunc, 500*time.Millisecond, 2)(ctx)
}
