// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/dut"
)

// HasDefaultKeyStatefulDiskLayout checks if the DUT has the "default-key-stateful" disk layout.
func HasDefaultKeyStatefulDiskLayout(ctx context.Context, dut *dut.DUT) bool {
	out, err := dut.Conn().CommandContext(ctx, "cros_config", "/disk-layout", "default-key-stateful").Output()
	if err != nil {
		// If cros_config returns an error, it means the device does not have the
		// default-key-stateful disk layout. Return false.
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
