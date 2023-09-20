// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FuseboxDirPath is the path to the directory containing all the fusebox mounts.
const FuseboxDirPath = "/media/fuse/fusebox"

// GenerateTestFileName generates a unique-ish file/folder name based on a provided
// prefix, the current time, and a random number.
func GenerateTestFileName(fName string) string {
	ext := filepath.Ext(fName)
	baseName := strings.TrimSuffix(fName, ext)
	return fmt.Sprintf("%s-%d-%d%s", baseName, time.Now().UnixNano(), rand.Intn(10000), ext)
}

// GetCopiedName returns the file name with the number suffix, which is the default
// rule of how Files app generate new file name if the one copied already exists.
// For example, a.txt -> a (1).txt
func GetCopiedName(fileName string, suffix int) string {
	ext := filepath.Ext(fileName)
	baseName := strings.TrimSuffix(fileName, ext)
	return fmt.Sprintf("%s (%d)%s", baseName, suffix, ext)
}

// CreateFileInFusebox creates a file in the fusebox volume identified by the fuseboxToken.
// This function returns the full path of the newly created file.
func CreateFileInFusebox(fuseboxToken, fileName, fileContent string) (string, error) {
	fullPath := filepath.Join(FuseboxDirPath, fuseboxToken, fileName)
	file, err := os.Create(fullPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create file %q", fullPath)
	}
	if _, err := file.WriteString(fileContent); err != nil {
		return "", errors.Wrapf(err, "failed to write into file %q", fullPath)
	}
	if err := file.Close(); err != nil {
		return "", errors.Wrapf(err, "failed to close file %q", fullPath)
	}
	return fullPath, nil
}

// CreateFolderInFusebox creates a folder in the fusebox volume identified by the fuseboxToken.
// This function returns the full path of the newly created folder.
func CreateFolderInFusebox(fuseboxToken, folderName string) (string, error) {
	fullPath := filepath.Join(FuseboxDirPath, fuseboxToken, folderName)
	if err := os.Mkdir(fullPath, 0755); err != nil {
		return "", errors.Wrapf(err, "failed to create folder %q", fullPath)
	}
	return fullPath, nil
}

// RemovePathInFusebox removes the specified path and use poll to check if the
// removal is successful or not.
// This function is mainly used in the cleanup phase, we want to give some time
// to the `RemoveAll` in case the fusebox unmounts before the removal finishes.
func RemovePathInFusebox(ctx context.Context, fullPath string, timeout time.Duration) {
	if err := os.RemoveAll(fullPath); err != nil {
		testing.ContextLog(ctx, "Failed removing path: ", err)
		return
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "failed to stat path")
		}
		return errors.New("still waiting for path to be removed")
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		testing.ContextLog(ctx, "Failed removing path from fusebox: ", err)
	}
}
