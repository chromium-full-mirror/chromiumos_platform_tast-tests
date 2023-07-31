// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package onedrive

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FindFilesOlderThanOneDay returns files older than 1 day in the `dir`.
func FindFilesOlderThanOneDay(dir string) (infos []os.FileInfo, err error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	oneDayAgo := time.Now().Add(-24 * time.Hour)
	for _, e := range dirEntries {
		info, err := e.Info()
		if (err == nil) && info.Mode().IsRegular() && (info.ModTime().Compare(oneDayAgo) < 0) {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

// DeleteOldRemoteFiles deletes test files older than 1 day that are siblings of dstFileName.
func DeleteOldRemoteFiles(ctx context.Context, dstFileName string) {
	// We don't know the fsp.?? directory name, so we use the `dstFileName` to find the file and find its parent.
	dstFile, err := filepath.Glob("/media/fuse/fusebox/fsp.*/" + dstFileName)
	if err != nil || len(dstFile) < 1 {
		testing.ContextLogf(ctx, "Failed to list old files directory: %s. %v", dstFileName, err)
		return
	}
	file := dstFile[0]
	dir := filepath.Dir(file)
	files, err := FindFilesOlderThanOneDay(dir)
	if err != nil {
		testing.ContextLog(ctx, "Failed to list old files: ", err)
		return
	}

	testFileName, _ := regexp.Compile(`Sample_(DOCX|PPTX|XLSX)_file.*$`)
	for _, f := range files {
		if !testFileName.MatchString(f.Name()) {
			continue
		}

		testing.ContextLog(ctx, "Deleting old file: ", f.Name())
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			testing.ContextLogf(ctx, "Failed to remove file: %q - %v", f.Name(), err)
		}
	}
}

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
