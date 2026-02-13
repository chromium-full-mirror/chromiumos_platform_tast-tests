// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// GetNumFans counts how many fans does the DUT have.
func GetNumFans(ctx context.Context) int {
	cmd := "ectool pwmgetnumfans | grep -o [0-9]"
	output, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
	if err != nil {
		testing.ContextLog(ctx, "Cannot get fan numbers from ectool: ", err)
		return 0
	}
	num, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		testing.ContextLog(ctx, "Failed to parse fan count: ", err)
		return 0
	}
	return int(num)
}

// ReadRpm reads the rpm of all fans and returns them as a list.
func ReadRpm(ctx context.Context) ([]int64, error) {
	var rpmData []int64
	output, err := testexec.CommandContext(ctx, "ectool", "pwmgetfanrpm", "all").Output()
	if err != nil {
		return rpmData, errors.Wrap(err, "unable to get RPM from ectool")
	}
	// Trim the last new line to avoid a trailing empty string.
	splitOutput := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, ln := range splitOutput {
		if strings.Contains(ln, "stalled") {
			rpmData = append(rpmData, 0)
			continue
		}
		numString := strings.TrimSpace(strings.Split(ln, ":")[1])
		num, err := strconv.ParseInt(numString, 10, 64)
		if err != nil {
			return rpmData, errors.Wrap(err, "unable to convert read RPM to int64")
		}
		rpmData = append(rpmData, num)
	}
	return rpmData, nil
}
