// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var hidNameRegex = regexp.MustCompile(`/([^/]+)/input/input\d+$`)

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

// FindPhysicalStylusBatteryLevel returns whether the stylus has a battery, and the battery level for the stylus if it exists.
// Note: It is possible for to return false if the stylus does not have a battery and not return an error.
func FindPhysicalStylusBatteryLevel(ctx context.Context) (bool, int, error) {
	_, info, err := findPhysicalStylusDevInfo(ctx)
	if err != nil {
		return false, -1, err
	}
	hidName, err := parseHIDName(ctx, info.sysfs)
	if err != nil {
		return false, -1, err
	}
	queryString := fmt.Sprintf("/sys/class/power_supply/hid-%s-battery", hidName)

	// Returns if no battery is present on stylus.
	if _, erra := os.Stat(queryString); erra != nil {
		return false, -1, nil
	}

	data, err := os.ReadFile(queryString + "/capacity")
	if err != nil {
		return false, -1, errors.Wrapf(err, "stylus has battery, but %s/capacity does not exist", queryString)
	}
	dataStr := string(data)
	levelStr := strings.TrimSuffix(dataStr, "\n")
	level, err := strconv.Atoi(levelStr)
	if err != nil {
		return false, -1, errors.Wrapf(err, "%s/capacity (%v) cannot be converted to an integer", queryString, dataStr)
	}
	return true, level, nil
}

// parseHIDName parses and returns the HID device name from a /sys/devices path.
func parseHIDName(ctx context.Context, sysfs string) (string, error) {
	ms := hidNameRegex.FindStringSubmatch(sysfs)
	if ms == nil {
		return "", errors.Errorf("could not parse HID name from %q", sysfs)
	}
	return ms[1], nil
}
