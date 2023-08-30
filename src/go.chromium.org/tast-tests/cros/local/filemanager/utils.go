// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
)

// GenerateTestFileName generates a unique-ish file/folder name based on a provided
// prefix, the current time, and a random number.
func GenerateTestFileName(fName string) string {
	ext := filepath.Ext(fName)
	baseName := strings.TrimSuffix(fName, ext)
	return fmt.Sprintf("%s-%d-%d%s", baseName, time.Now().UnixNano(), rand.Intn(10000), ext)
}

// CreateFileInFusebox creates a file in the fusebox volume identified by the fuseboxToken.
// This function returns the full path of the newly created file.
func CreateFileInFusebox(fuseboxToken, fileName, fileContent string) (string, error) {
	fullPath := fmt.Sprintf("/media/fuse/fusebox/%s/%s", fuseboxToken, fileName)
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
