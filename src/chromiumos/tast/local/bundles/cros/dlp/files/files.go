// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package files contains functionality shared by tests that
// exercise DLP files restrictions.
package files

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/cryptohome"
)

// ClearDownloads lists and deletes all the files in user's Downloads directory.
func ClearDownloads(ctx context.Context, cr *chrome.Chrome) error {
	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	files, err := ioutil.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to get files from Downloads directory")
	}
	for _, file := range files {
		if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
			return errors.Wrapf(err, "failed to remove file: %s", file.Name())
		}
	}
	return nil
}
