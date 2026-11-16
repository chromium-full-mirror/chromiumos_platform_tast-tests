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
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FindEntriesOlderThanHalfAnHour returns files and folders older than 30 minutes in the `dir`.
func FindEntriesOlderThanHalfAnHour(dir string) (infos []os.FileInfo, err error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	halfAnHourAgo := time.Now().Add(-30 * time.Minute)
	for _, e := range dirEntries {
		info, err := e.Info()
		if err == nil && (info.ModTime().Compare(halfAnHourAgo) < 0) {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

// DeleteOldRemoteFiles deletes test files older than 1 day that are siblings of dstFileName.
func DeleteOldRemoteFiles(ctx context.Context, dstFileName string) {
	// We don't know the fsp.?? directory name, so we use the `dstFileName` to find the file and find its parent.
	dstFile, err := filepath.Glob(filemanager.FuseboxDirPath + "/fsp.*/" + dstFileName)
	if err != nil || len(dstFile) < 1 {
		testing.ContextLogf(ctx, "Failed to list old files directory: %s. %v", dstFileName, err)
		return
	}
	file := dstFile[0]
	dir := filepath.Dir(file)
	files, err := FindEntriesOlderThanHalfAnHour(dir)
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

// DeleteFilesInRootByPrefix deletes files in root OneDrive folder that starts with `prefix`.
func DeleteFilesInRootByPrefix(ctx context.Context, prefix string) error {
	files, err := filepath.Glob(filemanager.FuseboxDirPath + "/fsp.*/" + prefix + "*")
	if err != nil {
		return errors.Wrap(err, "failed to list files by prefix")
	}

	for _, f := range files {
		testing.ContextLog(ctx, "Deleting file: ", f)
		if err := os.Remove(f); err != nil {
			return errors.Wrapf(err, "failed to remove file: %s", f)
		}
	}
	return nil
}

// DeleteDirsInRootByPrefixes deletes directories in the root OneDrive folder
// whose names start with any of the given prefixes.
func DeleteDirsInRootByPrefixes(ctx context.Context, prefixes ...string) {
	matches, err := filepath.Glob(filemanager.FuseboxDirPath + "/fsp.*")
	if err != nil {
		return
	}
	for _, fspDir := range matches {
		entries, err := os.ReadDir(fspDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(entry.Name(), prefix) {
					dirPath := filepath.Join(fspDir, entry.Name())
					testing.ContextLog(ctx, "Deleting stale OneDrive directory: ", dirPath)
					if err := os.RemoveAll(dirPath); err != nil {
						testing.ContextLogf(ctx, "Failed to delete directory %s: %v", dirPath, err)
					}
					break
				}
			}
		}
	}
}

// OpenOneDrive opens the Microsoft OneDrive root directory in the Files app by
// scrolling the StaticText label (rather than the outer TreeItem container)
// into view before clicking.
func OpenOneDrive(files *filesapp.FilesApp) uiauto.Action {
	dir := nodewith.Name(filesapp.OneDrive).Role(role.TreeItem).First()
	label := nodewith.Name(filesapp.OneDrive).Role(role.StaticText).Ancestor(dir).First()
	expectedTitle := filesapp.FilesTitlePrefix + filesapp.OneDrive
	return uiauto.Combine("open Microsoft OneDrive",
		files.ScrollToVisible(label),
		files.RetryUntil(
			uiauto.Combine("scroll and click Microsoft OneDrive",
				files.ScrollToVisible(label),
				files.LeftClick(label),
			),
			files.Exists(nodewith.Name(expectedTitle).Role(role.Window).First()),
		),
	)
}

// WaitForFileByPattern waits for a file or folder matching pattern to appear in
// the Files app listBox, scrolling through the virtualized list if needed, and
// scrolls the matched row into view.
func WaitForFileByPattern(ctx context.Context, files *filesapp.FilesApp, pattern *regexp.Regexp) error {
	filesBox := nodewith.Role(role.ListBox)
	fileRow := nodewith.NameRegex(pattern).Role(role.StaticText).Ancestor(filesBox).First()
	optionFinder := nodewith.Role(role.ListBoxOption).Ancestor(filesBox)
	lastBottomName := ""
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := files.Exists(fileRow)(ctx); err == nil {
			return nil
		}
		if options, err := files.NodesInfo(ctx, optionFinder); err == nil && len(options) > 0 {
			bottomName := options[len(options)-1].Name
			if bottomName == lastBottomName {
				// Reached the bottom of the virtualized list; scroll back to the top.
				_ = files.ScrollToVisible(optionFinder.First())(ctx)
				lastBottomName = ""
			} else {
				_ = files.ScrollToVisible(optionFinder.Nth(len(options) - 1))(ctx)
				lastBottomName = bottomName
			}
		}
		return errors.Errorf("entry matching %q not found in listBox yet", pattern.String())
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 2 * time.Second}); err != nil {
		return err
	}
	return files.ScrollToVisible(fileRow)(ctx)
}
