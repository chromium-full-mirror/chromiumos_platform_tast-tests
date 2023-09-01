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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// findEntriesOlderThanOneDay returns files and folders older than 1 day in the `dir`.
func findEntriesOlderThanOneDay(dir string) (infos []os.FileInfo, err error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	oneDayAgo := time.Now().Add(-24 * time.Hour)
	for _, e := range dirEntries {
		info, err := e.Info()
		if err == nil && (info.ModTime().Compare(oneDayAgo) < 0) {
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
	files, err := findEntriesOlderThanOneDay(dir)
	if err != nil {
		testing.ContextLog(ctx, "Failed to list old files: ", err)
		return
	}

	testFileName, _ := regexp.Compile(`Sample_(DOCX|PPTX|XLSX)_file.*$`)
	testDirName, _ := regexp.Compile(`^_odfs_.*$`)
	for _, f := range files {
		if !testFileName.MatchString(f.Name()) && !testDirName.MatchString(f.Name()) {
			continue
		}

		testing.ContextLog(ctx, "Deleting old file: ", f.Name())
		if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
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

// MaybeUnmountOdfs unmounts ODFS if it's mounted.
func MaybeUnmountOdfs(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch Files app to unmount ODFS")
	}
	defer filesApp.Close(cleanupCtx)

	volumeID, err := filesApp.GetVolumeID(ctx, cr, "Microsoft OneDrive")
	if err != nil {
		return errors.Wrap(err, "failed to get the volume ID for ODFS")
	}
	if volumeID == "" {
		testing.ContextLog(ctx, "Unmounting ODFS not needed")
		return nil
	}

	testing.ContextLog(ctx, "Unmounting ODFS")
	if err := filesApp.Unmount(cr, volumeID)(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount ODFS")
	}

	return nil
}
