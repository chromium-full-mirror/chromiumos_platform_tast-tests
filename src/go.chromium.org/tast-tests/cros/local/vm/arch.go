// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"runtime"
)

// TargetArch returns the name of the VM architecture that should be used
func TargetArch() string {
	if runtime.GOARCH == "arm64" {
		// For now, we ship the same VM image to arm64 as for arm devices
		return "arm"
	}
	return runtime.GOARCH
}
