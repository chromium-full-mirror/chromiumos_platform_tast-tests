// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains functionality shared by tests that
// exercise firmware.
package utils

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast/core/errors"
)

// VerifyFwIDs will show in logs the current firmware version and compare it to expected ones if they are provided.
func VerifyFwIDs(ctx context.Context, h *firmware.Helper, exROVersion, exRWVersion string) error {
	currentROID, err := GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		return err
	}
	if exROVersion != currentROID {
		return errors.Errorf("got %s RO version, but expected %s", currentROID, exROVersion)
	}
	currentRWID, err := GetFwVersion(ctx, h, reporters.CrossystemParamFwid)
	if err != nil {
		return err
	}
	if exRWVersion != currentRWID {
		return errors.Errorf("got %s RW version, but expected %s", currentRWID, exRWVersion)
	}
	return nil
}

// GetFwVersion accepts 'crossystem' params (i.e., CrossystemParamFwid & CrossystemParamRoFwid),
// splits the outputs from them and only returns the version numbers.
func GetFwVersion(ctx context.Context, h *firmware.Helper, param reporters.CrossystemParam) (string, error) {
	fwid, err := h.Reporter.CrossystemParam(ctx, param)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get only the fw id from crossystem: %v", param)
	}
	splitout := strings.Split(fwid, ".")
	if len(splitout) < 4 {
		return "", errors.Wrapf(err, "got invalid fw id from crossystem: %v", fwid)
	}
	onlyID := splitout[1] + "." + splitout[2] + "." + splitout[3]
	return onlyID, err
}
