// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/ssh"
)

// rootPartitionDeviceAndNumber detects the name of the root partition device and its number using a regex.
var rootPartitionDeviceAndNumber = regexp.MustCompile(`/dev/(?:(?:(\S+?\d)p)|(\S+[A-Za-z]))(\d+)$`)

// ReadBootID reads the current boot_id at hst.
func ReadBootID(ctx context.Context, hst *ssh.Conn) (string, error) {
	out, err := hst.CommandContext(ctx, "cat", "/proc/sys/kernel/random/boot_id").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadCurrentRootPartitionName reads the current root partition name at hst.
func ReadCurrentRootPartitionName(ctx context.Context, hst *ssh.Conn) (string, error) {
	currentRootPartitionBytes, err := hst.CommandContext(ctx, "rootdev", "-s").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(currentRootPartitionBytes)), nil
}

// SplitRootDevAndPart takes a rootdev path and splits the device and part num, inverse of RootDevPartitionPath.
// This copies implementation of the utility function from go.chromium.org/tast-tests/cros/local/kernel/kernel_utils.go
// TODO(b/344509814): Remove once kernel_utils.go is available from go.chromium.org/tast-tests/cros/remote/
func SplitRootDevAndPart(ctx context.Context, rootDevWithPart string) (string, int) {
	// Partition path looks like /dev/{device}p?{partition} with a 'p' between only if `device` ends with a digit.
	// e.g. /dev/mmcblk1p1 -> (/dev/mmcblk1, 1) and /dev/sda1 -> (/dev/sda, 1)
	match := rootPartitionDeviceAndNumber.FindStringSubmatch(rootDevWithPart)
	part, _ := strconv.Atoi(match[len(match)-1]) // Get just the partition number for current kernel copy.
	var device string
	// The device could be in match[1] or match[2] depending on format so iterate and find the non-empty one.
	for _, str := range match[1:] {
		if str != "" {
			device = str
			break
		}
	}
	rootDevWithoutPart := "/dev/" + device

	return rootDevWithoutPart, part
}
