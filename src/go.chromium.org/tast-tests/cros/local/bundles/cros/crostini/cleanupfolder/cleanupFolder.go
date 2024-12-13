// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cleanupfolder provides funcs to cleanup folders in ChromeOS.
package cleanupfolder

import (
	"os"
	"path/filepath"

	"go.chromium.org/tast/core/errors"
)

// RemoveAllFilesInDirectory removes all files in a directory but leaves the directory itself intact.
func RemoveAllFilesInDirectory(directory string) error {
	files, err := os.ReadDir(directory)
	if err != nil {
		return errors.Wrapf(err, "failed to read files in %s", directory)
	}
	for _, f := range files {
		path := filepath.Join(directory, f.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to RemoveAll(%q)", path)
		}
	}
	return nil
}
