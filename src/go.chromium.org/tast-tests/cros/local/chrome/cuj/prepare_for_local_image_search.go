// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
)

// PrepareImageSearchFiles copies files from a given source to the Downloads folder, repeating
// the process a given number of times. It returns cleanup and error. Remember to call the cleanup
// function to delete all the files created in this function.
func PrepareImageSearchFiles(ctx context.Context, user, sourceImageFilePath string, copyNumber int) (cleanup func() error, retErr error) {
	userPath, err := cryptohome.UserPath(ctx, user)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user's Home path")
	}
	downloadsPath, err := cryptohome.DownloadsPath(ctx, user)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user's Download path")
	}
	testPicture := launcher.ImageSearchPowerTestPictureName
	if _, err := os.Stat(sourceImageFilePath); err != nil {
		return nil, errors.Wrap(err, "failed to get file for local image search")
	}
	tmpFolderName := fmt.Sprintf("images_for_local_search_%s", time.Now().Format("2006-01-02T15:04"))
	tmpFolderPath := filepath.Join(userPath, tmpFolderName)
	if err := os.Mkdir(tmpFolderPath, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to make temporary folder")
	}
	defer os.RemoveAll(tmpFolderPath)

	for i := 0; i < copyNumber; i++ {
		imageFile := filepath.Join(tmpFolderPath, testPicture) + fmt.Sprintf("%d.png", i)
		if err := fsutil.CopyFile(sourceImageFilePath, imageFile); err != nil {
			return nil, errors.Wrapf(err, "failed to copy the test image to %s", tmpFolderPath)
		}
	}
	dstPath := filepath.Join(downloadsPath, tmpFolderName)
	if err := fsutil.CopyDir(tmpFolderPath, dstPath); err != nil {
		return nil, errors.Wrap(err, "failed to move temporary folder to Downloads")
	}

	cleanup = func() error {
		return os.RemoveAll(dstPath)
	}

	return cleanup, nil
}
