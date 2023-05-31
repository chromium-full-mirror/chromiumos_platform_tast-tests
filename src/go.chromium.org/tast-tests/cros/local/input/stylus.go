// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// findPhysicalStylusDevInfo iterates over devices and returns devinfo for the stylus if found
// otherwise returns boolean stating a physical stylus was not found.
func findPhysicalStylusDevInfo(ctx context.Context) (bool, *devInfo, error) {
	infos, err := readDevices("")
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to read devices")
	}

	for _, info := range infos {
		if info.isStylus() && info.phys != "" {
			testing.ContextLogf(ctx, "Found stylus %+v", info)
			return true, info, nil
		}
	}

	return false, nil, nil
}

// FindPhysicalStylus returns the path for the stylus's /dev/event entry, if present.
func FindPhysicalStylus(ctx context.Context) (bool, string, error) {
	success, info, err := findPhysicalStylusDevInfo(ctx)

	if info == nil {
		return success, "", err
	}
	return success, info.path, err
}
