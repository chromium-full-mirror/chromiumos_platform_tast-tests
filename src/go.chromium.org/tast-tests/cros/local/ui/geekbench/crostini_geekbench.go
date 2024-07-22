// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbench

import (
	"context"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// crostiniGeekbenchInfo contains information for running Geekbench in crostini VM.
var crostiniGeekbenchInfo = GBInfo{
	Name:           "crostini",
	createGBDir:    createCrostiniGBDir,
	pushGBFiles:    pushCrostiniGBFiles,
	pushGBLicense:  pushCrostiniGBLicense,
	getGBCommand:   getCrostiniGBCommand,
	retrieveGBFile: retrieveCrostiniGBFile,
}

func createCrostiniGBDir(ctx context.Context, cont *vm.Container, cr *chrome.Chrome, name string) (geekbenchDir, error) {
	gbDir := geekbenchDir{}
	if err := cont.Command(ctx, "mkdir", name).Run(testexec.DumpLogOnError); err != nil {
		return gbDir, errors.Wrap(err, "failed to create Geekbench folder")
	}

	remove := func() {
		// Remove the file from download folder.
		if err := cont.Command(ctx, "rm", "-r", name).Run(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "Failed to delete Geekbench folder: ", err)
		}
	}
	userName := strings.Split(cr.NormalizedUser(), "@")[0]
	gbFolderPath := filepath.Join("/home", userName, name)

	userHome, err := cont.GetHomeDir(ctx)
	if err != nil {
		return gbDir, errors.Wrap(err, "failed to find user home directory")
	}

	gbDir.path = gbFolderPath
	gbDir.home = userHome
	gbDir.rmDir = remove

	return gbDir, nil
}

func pushCrostiniGBFiles(ctx context.Context, cont *vm.Container, gbFiles geekbenchFiles) error {
	if err := cont.PushFile(ctx, gbFiles.binarySrc, gbFiles.binaryDest); err != nil {
		return errors.Wrap(err, "failed to push Geekbench binary")
	}
	if err := cont.PushFile(ctx, gbFiles.plarSrc, gbFiles.plarDest); err != nil {
		return errors.Wrap(err, "failed to push Geekbench plar file")
	}
	if gbFiles.workloadSrc != "" {
		if err := cont.PushFile(ctx, gbFiles.workloadSrc, gbFiles.workloadDest); err != nil {
			return errors.Wrap(err, "failed to push Geekbench workload file")
		}
	}
	if err := cont.Command(ctx, "chmod", "0755", gbFiles.binaryDest).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to change execute permission")
	}
	return nil
}

func pushCrostiniGBLicense(ctx context.Context, cont *vm.Container, licensePath, licenseStr string) (func(), error) {
	if err := cont.WriteFile(ctx, licensePath, licenseStr); err != nil {
		return nil, errors.Wrap(err, "failed to write Geekbench license file")
	}

	rmLicense := func() {
		if err := cont.Cleanup(ctx, licensePath); err != nil {
			testing.ContextLog(ctx, "Failed to delete Geekbench license: ", err)
		}
	}
	return rmLicense, nil
}

func getCrostiniGBCommand(ctx context.Context, cont *vm.Container, execFilePath, resultPath string) *testexec.Cmd {
	return cont.Command(ctx, execFilePath, "--no-upload", "--export-json", resultPath)
}

func retrieveCrostiniGBFile(ctx context.Context, cont *vm.Container, resultPath, logFilePath string) error {
	if err := cont.GetFile(ctx, resultPath, logFilePath); err != nil {
		return errors.Wrap(err, "failed to get file from container")
	}
	return nil
}
