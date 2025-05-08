// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
)

// DDCommand function will run dd with specified parameters.
func DDCommand(ctx context.Context, dut *dut.DUT, ifParam, ofParam, bsParam string, countParam, skipParam int) ([]byte, error) {

	ofStr := ""
	if ofParam != "" {
		ofStr = fmt.Sprintf("of=%v ", ofParam)
	}

	bsStr := ""
	if bsParam != "" {
		bsStr = fmt.Sprintf("bs=%v ", bsParam)
	}

	cmd := fmt.Sprintf("dd if=%v %v%vcount=%v", ifParam, ofStr, bsStr, countParam)

	if skipParam > 0 {
		cmd += fmt.Sprintf(" skip=%v", skipParam)
	}

	return RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
}

// GetStatefulPartition returns full device path to the stateful partition.
// (e.g., /dev/nvme0n1 -> /dev/nvme0n1p1) by appending 'p' before the partition number and +1.
func GetStatefulPartition(ctx context.Context, dut *dut.DUT) (string, error) {
	// Get root device.
	disk, err := GetInternalStorage(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "failed to get internal disk")
	}

	// If the device ends in a digit, a "p" appears before the partition number then add "1".
	statefulDev := disk.Path
	if unicode.IsDigit(rune(statefulDev[len(statefulDev)-1])) {
		statefulDev += "p"
	}
	return statefulDev + "1", nil
}

// GetSkipSize returns physical offset as bytes offset.
// It extracts the physical offset from the fourth line of `filefrag -v` output
// and converts it to an integer.
func GetSkipSize(ctx context.Context, dut *dut.DUT, path string) (int, error) {

	// Getting physical offset.
	cmd := fmt.Sprintf("filefrag -v %s | awk 'NR==4 {print $4}' | cut -d. -f1", path)
	output, err := RunCmdWithStringOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get physical offset")
	}
	offsetStr := strings.TrimSpace(output)
	physOffset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert offset into int")
	}
	// Returning skip size into bytes.
	return physOffset * 4096, nil
}

// GetMD5 returns the hash for the file contents of the given path.
func GetMD5(ctx context.Context, dut *dut.DUT, path string, skipSize int) ([16]byte, error) {
	var hash [16]byte
	// Getting File Hash.
	out, err := DDCommand(ctx, dut, path, "", "1", 100, skipSize)
	if err != nil {
		return hash, errors.Wrap(err, "failed to get hash")
	}
	hash = md5.Sum(out)
	return hash, nil
}

// RebootDUT function reboot the device and wait for the comeup.
func RebootDUT(ctx context.Context, dut *dut.DUT) error {

	// Rebooting device.
	err := dut.Reboot(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to reboot")
	}

	// Wait for DUT to come back online.
	if err = dut.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to reconnect to device after reboot")
	}
	return nil
}

// VerifyFileContent function validates whether the expected
// and actual file contents are the same.
func VerifyFileContent(ctx context.Context, dut *dut.DUT, filePath, expected string) error {

	// Reading file content.
	cmd := fmt.Sprintf("cat %s | tr -d '\n'", filePath)
	out, err := RunCmdWithStringOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to read file")
	}

	// Comparing the file content.
	if expected != out {
		return errors.Errorf("expected file content is %s and actual content is %s", expected, out)
	}
	return nil
}
