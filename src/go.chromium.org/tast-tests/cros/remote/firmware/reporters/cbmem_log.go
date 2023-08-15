// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporters

import (
	"context"
	"regexp"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast/core/errors"
)

// GetCBMEMLogs gets CBMEM log from the last boot.
func (r *Reporter) GetCBMEMLogs(ctx context.Context) (string, error) {
	cbmem, err := r.CommandOutput(ctx, "cbmem", "-1")
	if err != nil {
		return "", errors.Wrap(err, "failed to get CBMEM logs")
	}
	return cbmem, nil
}

// GetDisplayedFWScreens gets the CBMEM logs, and returns a list of all the
// recorded firmware screens, specifically their ids.
func (r *Reporter) GetDisplayedFWScreens(ctx context.Context) ([]firmware.FwScreenID, error) {
	cbmemLogs, err := r.GetCBMEMLogs(ctx)
	if err != nil {
		return nil, err
	}
	reScreenID := regexp.MustCompile(`[vb2ex_display_ui|vboot_draw_|ui_display].*screen=(\w+).*[\n\r]`)

	var foundScreens []firmware.FwScreenID
	matches := reScreenID.FindAllStringSubmatch(cbmemLogs, -1)
	for _, match := range matches {
		fwScreenID, err := strconv.ParseInt(match[1], 0, 0)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse firmware screen id")
		}
		foundScreens = append(foundScreens, firmware.FwScreenID(fwScreenID))
	}
	return foundScreens, nil
}

// CheckDisplayedScreens uses reporter to obtain a list of firmware screen ids
// recorded in the CBMEM logs, and verifies if there are matches found for the
// passed-in list.
func (r *Reporter) CheckDisplayedScreens(ctx context.Context, expected []firmware.FwScreenID) (bool, error) {
	fwScreens, err := r.GetDisplayedFWScreens(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get firmware screens")
	}
	removeAdjacentDuplicates := func(fwScreens *[]firmware.FwScreenID) {
		for i := 1; i < len(*fwScreens); i++ {
			if (*fwScreens)[i-1] == (*fwScreens)[i] {
				copy((*fwScreens)[i:], (*fwScreens)[i+1:])
				*fwScreens = (*fwScreens)[:len(*fwScreens)-1]
				i--
			}
		}
	}
	removeAdjacentDuplicates(&fwScreens)
	return cmp.Equal(fwScreens, expected), nil
}
