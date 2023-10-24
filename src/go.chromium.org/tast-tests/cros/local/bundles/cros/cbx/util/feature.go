// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains utilities for cbx related testing.
package util

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/errors"
)

// CbxFeatures returns an array of features which are specifically for CBX
// devices. These values should be surfaced via a `FeatureManagement` prefixed
// flag in `feature_check`.
func CbxFeatures() []string {
	return []string{
		"16Desks",
		"DriveFsBulkPinning",
		"OobeSimon",
		"TimeOfDayScreenSaver",
		"TimeOfDayWallpaper",
		"FeatureAwareDeviceDemoMode",
		"VideoConference",
	}
}

// Feature check if the DUT has a specified CBX feature.
func Feature(ctx context.Context, feature string) (has bool, err error) {
	cmd := testexec.CommandContext(ctx,
		"/usr/sbin/feature_check",
		fmt.Sprintf("--feature_name=%s", feature),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, errors.Wrapf(err, "failed to call /usr/sbin/feature_check: %s", string(out))
	}
	strOut := strings.TrimSpace(string(out))
	switch strOut {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, errors.Errorf("unexpected output from /usr/sbin/feature_check: %s", strOut)
	}
}

// FeatureLevel returns the feature level number of the DUT.
func FeatureLevel(ctx context.Context) (level int64, err error) {
	cmd := testexec.CommandContext(ctx,
		"feature_explorer",
		"--feature_level",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to call feature_explorer %s", string(out))
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 0, 64)
}
