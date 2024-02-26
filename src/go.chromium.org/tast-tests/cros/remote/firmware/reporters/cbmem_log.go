// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporters

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CBMEMLogType is a string represents the coverage of DUT's firmware log.
type CBMEMLogType string

// Firmware log types to parse to cbmem command.
const (
	LastBootLog         CBMEMLogType = "-1"
	SecondToLastBootLog CBMEMLogType = "-2"
	ConsoleLog          CBMEMLogType = "-c"
)

// GetCBMEMLogs gets CBMEM log from the last boot.
func (r *Reporter) GetCBMEMLogs(ctx context.Context, logType ...CBMEMLogType) (string, error) {
	var logTypeString string
	switch len(logType) {
	case 0:
		logTypeString = string(LastBootLog)
	case 1:
		logTypeString = string(logType[0])
	default:
		return "", errors.New("too many arguments")
	}
	cbmem, err := r.CommandOutput(ctx, "cbmem", logTypeString)
	if err != nil {
		return "", errors.Wrap(err, "failed to get CBMEM logs")
	}
	return cbmem, nil
}

// GetCBMEMTimestamps gets CBMEM timestamps.
func (r *Reporter) GetCBMEMTimestamps(ctx context.Context) (string, error) {
	timeStamps, err := r.CommandOutput(ctx, "cbmem", "-t")
	if err != nil {
		return "", errors.Wrap(err, "failed to get CBMEM timestamps")
	}
	return timeStamps, nil
}

// GetDisplayedFWScreens gets the CBMEM logs, and returns a list of all the
// recorded firmware screens, specifically their ids.
func (r *Reporter) GetDisplayedFWScreens(ctx context.Context, logType ...CBMEMLogType) ([]firmware.FwScreenID, error) {
	cbmemLogs, err := r.GetCBMEMLogs(ctx, logType...)
	if err != nil {
		return nil, err
	}

	screenWithNoID := "VbDisplayDebugInfo"
	reScreenID := regexp.MustCompile(
		`[vb2ex_display_ui|vboot_draw_|ui_display].*screen=(\w+).*[\n\r]` + `|` + screenWithNoID)

	var foundScreens []firmware.FwScreenID
	matches := reScreenID.FindAllStringSubmatch(cbmemLogs, -1)
	var prevMatch string
	var prevMatchFwScreenID firmware.FwScreenID
	for _, match := range matches {
		var fwScreenID firmware.FwScreenID
		switch match[0] {
		case screenWithNoID:
			fwScreenID = firmware.LegacyDebugInfo
		default:
			id, err := strconv.ParseInt(match[1], 0, 0)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse firmware screen id")
			}
			fwScreenID = firmware.FwScreenID(id)
		}
		// For devices with firmware version < 12045 (CL:1548301), pressing tab
		// prints the debug info directly on the firmware screen, instead of
		// launching a new page to display the info. When this is the case,
		// the id of this firmware screen would get recorded twice in the cbmem
		// logs. Change the duplicate record to firmware.LegacyDebugInfo.
		matchPrefix := strings.Split(match[0], ":")[0]
		if matchPrefix != "vb2ex_display_ui" && prevMatch != "" && fwScreenID == prevMatchFwScreenID {
			// For LCUI machines, the duplicate record starts with the
			// "vboot_draw_ui" prefix. For LMUI machines, the exact same
			// log is printed again.
			if match[0] == prevMatch || matchPrefix == "vboot_draw_ui" {
				fwScreenID = firmware.LegacyDebugInfo
			}
		}
		foundScreens = append(foundScreens, fwScreenID)
		prevMatch = match[0]
		prevMatchFwScreenID = fwScreenID
	}
	return foundScreens, nil
}

// CheckDisplayedScreens uses reporter to obtain a list of firmware screen ids
// recorded in the CBMEM logs, and verifies if there are matches found for the
// passed-in list.
func (r *Reporter) CheckDisplayedScreens(ctx context.Context, expected []firmware.FwScreenID, logType ...CBMEMLogType) (bool, error) {
	fwScreens, err := r.GetDisplayedFWScreens(ctx, logType...)
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
	testing.ContextLogf(ctx, "Found firmware screens: %x", fwScreens)
	return cmp.Equal(fwScreens, expected), nil
}
