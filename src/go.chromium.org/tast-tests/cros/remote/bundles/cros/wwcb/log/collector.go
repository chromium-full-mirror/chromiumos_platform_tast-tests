// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package log used to do collecte logs excution function.
package log

import (
	"context"
	"os"
	"path"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
)

// CollectedLogs collecteds the list of DUT devices and check WWCB fixtures are alive.
func CollectedLogs(ctx context.Context, dut *dut.DUT, dutOutDir string) error {
	const dmesg, lsusb = "dmesg", "lsusb"
	if err := dumpLogsToFile(ctx, dut, dmesg, path.Join(dutOutDir, dmesg)); err != nil {
		return errors.Wrap(err, "failed to dump dmesg")
	}
	if err := dumpLogsToFile(ctx, dut, lsusb, path.Join(dutOutDir, lsusb)); err != nil {
		return errors.Wrap(err, "failed to dump lsusb")
	}
	if err := utils.TestAllFixtures(ctx); err != nil {
		return errors.Wrap(err, "failed to test all fixtures")
	}
	return nil
}

func CollectCrashReports(ctx context.Context, dut *dut.DUT) (int, error) {
	const crashReport = "/var/spool/crash"
	out,err := dut.Conn().CommandContext(ctx, "sudo", "ls", "/var/spool/crash").Output(exec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "failed to list /var/spool/crash")
	}
	var filesToCopy []string
	for _, file := range strings.Split(string(out), "\n") {
		if file != "" {
			filesToCopy = append(filesToCopy, path.Join(crashReport, file))
		}
	}
	fileutils.CopyFromDUTToHost(ctx, dut, filesToCopy)
	return len(filesToCopy), nil
}

func dumpLogsToFile(ctx context.Context, dut *dut.DUT, cmd, filePath string) error {
	out, err := dut.Conn().CommandContext(ctx, cmd).Output(exec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to executed %s", cmd)
	}
	sysInfo, err := os.Create(filePath)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", filePath)
	}
	defer sysInfo.Close()
	if _, err := sysInfo.Write(out); err != nil {
		return errors.Wrapf(err, "failed to write data %s", filePath)
	}
	return nil
}
