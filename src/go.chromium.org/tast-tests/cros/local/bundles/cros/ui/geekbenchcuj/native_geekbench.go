// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbenchcuj

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// nativeGeekbenchInfo contains information for running Geekbench in native ChromeOS.
var nativeGeekbenchInfo = geekbenchInfo{
	name:           "native",
	createGBDir:    createNativeGBDir,
	pushGBFiles:    pushNativeGBFiles,
	pushGBLicense:  pushNativeGBLicense,
	retrieveGBFile: retrieveNativeGBFile,
}

func createNativeGBDir(ctx context.Context, cont *vm.Container, cr *chrome.Chrome, name string) (geekbenchDir, error) {
	gbFolderPath := fmt.Sprintf("/usr/local/%s", name)
	if _, err := os.Stat(gbFolderPath); !os.IsNotExist(err) {
		testing.ContextLogf(ctx, "workspace directory %s already exists, removing", gbFolderPath)
		if err := os.RemoveAll(gbFolderPath); err != nil {
			return geekbenchDir{}, errors.Wrap(err, "failed to remove workspace directory")
		}
	}
	if err := os.Mkdir(gbFolderPath, 0755); err != nil {
		return geekbenchDir{}, errors.Wrap(err, "failed to create workspace directory")
	}

	remove := func(ctx context.Context) {
		os.RemoveAll(gbFolderPath)
	}

	return geekbenchDir{path: gbFolderPath, rmDir: remove}, nil
}

func pushNativeGBFiles(ctx context.Context, cont *vm.Container, gbFiles geekbenchFiles) error {
	if err := fsutil.CopyFile(gbFiles.binarySrc, gbFiles.binaryDest); err != nil {
		return errors.Wrap(err, "failed to push geekbench binary")
	}
	if err := fsutil.CopyFile(gbFiles.plarSrc, gbFiles.plarDest); err != nil {
		return errors.Wrap(err, "failed to push geekbench plar file")
	}
	if gbFiles.workloadSrc != "" {
		if err := fsutil.CopyFile(gbFiles.workloadSrc, gbFiles.workloadDest); err != nil {
			return errors.Wrap(err, "failed to push geekbench workload file")
		}
	}
	if err := os.Chmod(gbFiles.binaryDest, 0755); err != nil {
		return errors.Wrap(err, "failed to change execute permission")
	}

	return nil
}

func pushNativeGBLicense(ctx context.Context, cont *vm.Container, licensePath, licenseStr string) error {
	if err := os.WriteFile(licensePath, []byte(licenseStr), fs.ModeAppend); err != nil {
		return errors.Wrap(err, "failed to write Geekbench license file")
	}
	return nil
}

func retrieveNativeGBFile(ctx context.Context, cont *vm.Container, resultPath, logFilePath string) error {
	if err := fsutil.MoveFile(resultPath, logFilePath); err != nil {
		return errors.Wrap(err, "failed to get file from DUT")
	}
	return nil
}
