// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sysutil

import (
	"context"
	"strings"

	"chromiumos/tast/dut"
	"chromiumos/tast/ssh/linuxssh"
)

// IsRunningOnVM returns true if the test is running under a VM.
// Please do not use this to skip running your test entirely.
// Instead, introduce a new dependency describing the required feature:
// https://chromium.googuesource.com/chromiumos/platform/tast/+/HEAD/docs/test_dependencies.md
func IsRunningOnVM(ctx context.Context, d *dut.DUT) bool {
	const sysVendor = "/sys/devices/virtual/dmi/id/sys_vendor"
	// Currently only checks for QEMU but more kinds can be added here in the
	// future.
	const vmVendor = "QEMU"
	vendor, err := linuxssh.ReadFile(ctx, d.Conn(), sysVendor)
	if err != nil {
		// Assume that the read failed because the remote file doesn't exist;
		// which indicates that we're not running on a VM.
		return false
	}
	return strings.TrimSuffix(string(vendor), "\n") == vmVendor
}
