// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vm is a package that provides a function to check if tests are running under QEMU.
package vm

import (
	"io/ioutil"
	"path/filepath"
	"strings"
)

// isVM true if the test is running under QEMU or GCE.
var isVM bool

func init() {
	const dmiDir = "/sys/devices/virtual/dmi/id"
	productPath := filepath.Join(dmiDir, "product_name")
	vendorPath := filepath.Join(dmiDir, "sys_vendor")

	content, err := ioutil.ReadFile(productPath)
	if err != nil {
		isVM = false
		return
	}
	product := strings.TrimSpace(string(content))
	if product == "Google Compute Engine" {
		isVM = true
		return
	}

	content, err = ioutil.ReadFile(vendorPath)
	if err != nil {
		isVM = false
		return
	}
	vendor := strings.TrimSpace(string(content))
	isVM = vendor == "QEMU"
}

// IsRunningOnVM returns true if the test is running under QEMU.
// Please do not use this to skip running your test entirely.
// Instead, introduce a new dependency describing the required feature:
// https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_dependencies.md
func IsRunningOnVM() bool {
	return isVM
}
