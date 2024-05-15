// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/ssh"
)

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
