// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sysutil

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/ssh/linuxssh"
)

// IsRunningOnVM returns true if the test is running under a VM.
// Please do not use this to skip running your test entirely.
// Instead, introduce a new dependency describing the required feature:
// https://chromium.googuesource.com/chromiumos/platform/tast/+/HEAD/docs/test_dependencies.md
func IsRunningOnVM(ctx context.Context, d *dut.DUT) bool {
	const dmiDir = "/sys/devices/virtual/dmi/id"
	productPath := filepath.Join(dmiDir, "product_name")
	vendorPath := filepath.Join(dmiDir, "sys_vendor")

	// Check the product name to determine if running under VMLab, which
	// runs on GCE.
	product, err := linuxssh.ReadFile(ctx, d.Conn(), productPath)
	if err != nil {
		// Assume not a VM.
		return false
	}
	if string(product) == "Google Compute Engine\n" {
		return true
	}

	// Check the system vendor to determine if running under QEMU.
	vendor, err := linuxssh.ReadFile(ctx, d.Conn(), vendorPath)
	if err != nil {
		// Assume not a VM.
		return false
	}
	return string(vendor) == "QEMU\n"
}
