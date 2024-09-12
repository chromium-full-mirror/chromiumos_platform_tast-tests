// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crosconfig provides utilities to get camera related crosconfig
package crosconfig

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/crosconfig"
)

// HasMIPICamera check if the model has mipi camera through cros_config.
func HasMIPICamera(ctx context.Context) bool {
	for i := 0; ; i++ {
		devicePath := fmt.Sprintf("/camera/devices/%v", i)
		cameraType, err := crosconfig.Get(ctx, devicePath, "interface")
		if crosconfig.IsNotFound(err) {
			break
		}
		if cameraType == "mipi" {
			return true
		}
	}
	return false
}
