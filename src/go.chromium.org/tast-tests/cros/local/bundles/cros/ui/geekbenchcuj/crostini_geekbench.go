// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package geekbenchcuj

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// crostiniGeekbenchInfo contains information for running Geekbench in native ChromeOS.
var crostiniGeekbenchInfo = geekbenchInfo{
	name:           "crostini",
	createGBDir:    createCrostiniGBDir,
	pushGBFiles:    pushCrostiniGBFiles,
	pushGBLicense:  pushCrostiniGBLicense,
	retrieveGBFile: retrieveCrostiniGBFile,
}

func createCrostiniGBDir(ctx context.Context, cont *vm.Container, cr *chrome.Chrome, name string) (geekbenchDir, error) {
	gbDir := geekbenchDir{}
	if err := cont.Command(ctx, "mkdir", name).Run(testexec.DumpLogOnError); err != nil {
		return gbDir, errors.Wrap(err, "failed to create Geekbench folder")
	}

	remove := func(ctx context.Context) {
		// Remove the file from download folder.
		if err := cont.Command(ctx, "rm", "-r", name).Run(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "Failed to delete Geekbench folder: ", err)
		}
	}
	userName := strings.Split(cr.NormalizedUser(), "@")[0]
	gbFolderPath := filepath.Join("/home", userName, name)
	gbDir.path = gbFolderPath
	gbDir.rmDir = remove
	return gbDir, nil
}

func pushCrostiniGBFiles(ctx context.Context, cont *vm.Container, gbFiles geekbenchFiles) error {
	if err := cont.PushFile(ctx, gbFiles.binarySrc, gbFiles.binaryDest); err != nil {
		return errors.Wrap(err, "failed to push geekbench binary")
	}
	if err := cont.PushFile(ctx, gbFiles.plarSrc, gbFiles.plarDest); err != nil {
		return errors.Wrap(err, "failed to push geekbench plar file")
	}
	if gbFiles.workloadSrc != "" {
		if err := cont.PushFile(ctx, gbFiles.workloadSrc, gbFiles.workloadDest); err != nil {
			return errors.Wrap(err, "failed to push geekbench workload file")
		}
	}
	if err := cont.Command(ctx, "chmod", "0755", gbFiles.binaryDest).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to change execute permission")
	}
	return nil
}

func pushCrostiniGBLicense(ctx context.Context, cont *vm.Container, licensePath, licenseStr string) error {
	if err := cont.WriteFile(ctx, fmt.Sprintf("'%s'", licensePath), licenseStr); err != nil {
		return errors.Wrap(err, "failed to write Geekbench license file")
	}
	return nil
}

func retrieveCrostiniGBFile(ctx context.Context, cont *vm.Container, resultPath, logFilePath string) error {
	if err := cont.GetFile(ctx, resultPath, logFilePath); err != nil {
		return errors.Wrap(err, "failed to get file from container")
	}
	return nil
}
