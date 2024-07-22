// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbench

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// nativeGeekbenchInfo contains information for running Geekbench in native ChromeOS.
var nativeGeekbenchInfo = GBInfo{
	Name:           "native",
	createGBDir:    createNativeGBDir,
	pushGBFiles:    pushNativeGBFiles,
	pushGBLicense:  pushNativeGBLicense,
	getGBCommand:   getNativeGBCommand,
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

	userHome, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return geekbenchDir{}, errors.Wrap(err, "failed to find user home directory")
	}

	remove := func() {
		os.RemoveAll(gbFolderPath)
	}

	return geekbenchDir{path: gbFolderPath, home: userHome, rmDir: remove}, nil
}

func pushNativeGBFiles(ctx context.Context, cont *vm.Container, gbFiles geekbenchFiles) error {
	if err := fsutil.CopyFile(gbFiles.binarySrc, gbFiles.binaryDest); err != nil {
		return errors.Wrap(err, "failed to push Geekbench binary")
	}
	if err := fsutil.CopyFile(gbFiles.plarSrc, gbFiles.plarDest); err != nil {
		return errors.Wrap(err, "failed to push Geekbench plar file")
	}
	if gbFiles.workloadSrc != "" {
		if err := fsutil.CopyFile(gbFiles.workloadSrc, gbFiles.workloadDest); err != nil {
			return errors.Wrap(err, "failed to push Geekbench workload file")
		}
	}
	if err := os.Chmod(gbFiles.binaryDest, 0755); err != nil {
		return errors.Wrap(err, "failed to change execute permission")
	}

	return nil
}

func pushNativeGBLicense(ctx context.Context, cont *vm.Container, licensePath, licenseStr string) (func(), error) {
	if err := os.WriteFile(licensePath, []byte(licenseStr), 0755); err != nil {
		return nil, errors.Wrap(err, "failed to write Geekbench license file")
	}

	rmLicense := func() {
		if err := os.Remove(licensePath); err != nil {
			testing.ContextLog(ctx, "Failed to delete Geekbench license: ", err)
		}
	}

	return rmLicense, nil
}

func getNativeGBCommand(ctx context.Context, cont *vm.Container, execFilePath, resultPath string) *testexec.Cmd {
	cm, _ := testexec.CommandContextUser(ctx, "chronos", execFilePath, "--no-upload", "--export-json", resultPath)
	return cm
}

func retrieveNativeGBFile(ctx context.Context, cont *vm.Container, resultPath, logFilePath string) error {
	if err := fsutil.MoveFile(resultPath, logFilePath); err != nil {
		return errors.Wrap(err, "failed to get file from DUT")
	}
	return nil
}
