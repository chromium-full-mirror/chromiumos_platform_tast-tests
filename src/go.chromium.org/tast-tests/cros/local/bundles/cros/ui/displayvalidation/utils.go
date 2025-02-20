// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package displayvalidation provides info for the display validation test.
package displayvalidation

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DisplayResolution contains information about the resolution of a display.
type DisplayResolution struct {
	Width  uint64
	Height uint64
}

// DisplayInfo contains display size and resolution information.
type DisplayInfo struct {
	DisplayDiagonalSize float64
	Resolution          DisplayResolution
}

// Standard resolutions
var qxga = DisplayResolution{1536, 2048}
var wxga768 = DisplayResolution{1280, 768}
var wxga800 = DisplayResolution{1280, 800}
var fhd = DisplayResolution{1920, 1080}
var fhdPlus = DisplayResolution{2160, 1440}
var fhd1280 = DisplayResolution{1920, 1280}
var wuxga = DisplayResolution{1920, 1200}
var wqxga = DisplayResolution{2560, 1600}
var shd = DisplayResolution{1280, 720}
var qhd = DisplayResolution{2560, 1440}
var qhdPlus = DisplayResolution{3200, 1800}
var k4KUHD = DisplayResolution{3840, 2160}
var k2K = DisplayResolution{2256, 1504}
var hdPlus = DisplayResolution{1600, 900}

// Chromebook specific resolutions
var eve = DisplayResolution{2400, 1600}
var link = DisplayResolution{2560, 1700}
var nocturne = DisplayResolution{3000, 2000}

// A mirror of lcd_display_configs table in
// https://source.chromium.org/chromium/chromium/src/+/main:ui/display/types/display_constants.h
// Please keep the two tables in sync, any updates here should also be updated in chromium.
var knownDisplays = []DisplayInfo{
	{9.7, qxga},
	{10., wxga800},
	{10.1, wxga800},
	{10.1, fhd},
	{10.1, wuxga},
	{10.5, wuxga},
	{11.6, wxga768},
	{11.6, shd},
	{11.6, fhd},
	{12., fhd},
	{12.1, wxga800},
	{12.2, wuxga},
	{12.2, fhd},
	{12.3, qhd},
	{13.0, fhd},
	{13.1, k4KUHD},
	{13.3, wxga768},
	{13.3, fhd},
	{13.3, k2K},
	{13.3, k4KUHD},
	{13.5, fhd},
	{13.5, fhd1280},
	{13.5, k2K},
	{13.6, k2K},
	{14., wxga768},
	{14., fhd},
	{14., wuxga},
	{14., wqxga},
	{14., k4KUHD},
	{15.6, wxga768},
	{15.6, wuxga},
	{15.6, fhd},
	{15.6, k4KUHD},
	{17., hdPlus},
	{17., fhd},
	{17.3, fhd},
	{18.51, wxga768},

	// Non standard panel
	{11.0, fhdPlus},
	{12., DisplayResolution{1366, 912}},
	{12.3, eve},
	{12.85, link},
	{12.3, nocturne},
	{13.3, qhdPlus},

	// Chromebase
	{19.5, hdPlus},
	{21.5, fhd},
	{23.8, fhd},
}

// Used to fetch display info from CrOS health tool.
type displayBuffer struct {
	EDP embeddedDisplayInfo `json:"embedded_display"`
}

type embeddedDisplayInfo struct {
	DisplayWidth         *jsontypes.Uint32 `json:"display_width"`
	DisplayHeight        *jsontypes.Uint32 `json:"display_height"`
	ResolutionHorizontal *jsontypes.Uint32 `json:"resolution_horizontal"`
	ResolutionVertical   *jsontypes.Uint32 `json:"resolution_vertical"`
}

// This regex will match the first detailed timing description and extract
// the resolution and physical dimension.
// Ex from xol:
// DTD 1: 1920x1080 59.997 Hz 16:9   66.716 kHz 138.770 MHz (334 mm x 194 mm)
var detailedTimingDescriptionRegexp = regexp.MustCompile(`DTD \d:\s*(\d*)x(\d*).*\((\d*)\s*mm\s*x\s*(\d*).*mm\)`)

// ValidateEdidDisplayInfo checks if the display described by the edid is a known display.
func ValidateEdidDisplayInfo(ctx context.Context) (DisplayInfo, error) {
	// Get display information from EDID
	edidPath := []string{"/sys/class/drm/card0-eDP-1/edid", "/sys/class/drm/card0-DSI-1/edid", "/sys/class/drm/card1-eDP-1/edid"}
	var edid string
	// The edid info could be empty, so it's possible that the edid-decode fails. We don't need to report these failures.
	for _, path := range edidPath {
		if b, err := testexec.CommandContext(ctx, "edid-decode", path).Output(testexec.DumpLogOnError); err == nil {
			edid = string(b)
			break
		}
	}

	if edid == "" {
		return DisplayInfo{}, errors.New("failed to fetch edid information")
	}

	match := detailedTimingDescriptionRegexp.FindStringSubmatch(edid)
	if match == nil {
		return DisplayInfo{}, errors.New("failed to parse edid to get display information")
	}

	resW, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to parse resolution width from edid")
	}

	resH, err := strconv.ParseUint(match[2], 10, 32)
	if err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to parse resolution height from edid")
	}

	width, err := strconv.ParseUint(match[3], 10, 32)
	if err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to parse display width from edid")
	}

	height, err := strconv.ParseUint(match[4], 10, 32)
	if err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to parse display height from edid")
	}

	diagonal := math.Hypot(float64(width), float64(height)) / 25.4
	resolution := DisplayResolution{resW, resH}
	for _, display := range knownDisplays {
		if math.Abs(display.DisplayDiagonalSize-diagonal) < 0.1 && display.Resolution == resolution {
			return display, nil
		}
	}

	return DisplayInfo{diagonal, resolution}, errors.New("failed to match edid to a known display")
}

// ValidateCrOSHealthDisplayInfo checks if the display fetched from CrOS health tool is a known display.
func ValidateCrOSHealthDisplayInfo(ctx context.Context) (DisplayInfo, error) {
	// Ensure cros_healthd is running.
	if err := upstart.EnsureJobRunning(ctx, "cros_healthd"); err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to start cros_healthd")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return DisplayInfo{}, errors.New("failed to get the out directory")
	}

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryDisplay}
	displayRaw, err := croshealthd.RunTelem(ctx, params, outDir)
	if err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to fetch display data from cros_healthd")
	}

	var displayResult displayBuffer
	if err := json.Unmarshal(displayRaw, &displayResult); err != nil {
		return DisplayInfo{}, errors.Wrap(err, "failed to parse display data from cros_healthd")
	}

	diagonal := math.Hypot(float64(*displayResult.EDP.DisplayWidth), float64(*displayResult.EDP.DisplayHeight)) / 25.4
	resolution := DisplayResolution{uint64(*displayResult.EDP.ResolutionHorizontal), uint64(*displayResult.EDP.ResolutionVertical)}
	for _, display := range knownDisplays {
		if math.Abs(display.DisplayDiagonalSize-diagonal) < 0.1 && display.Resolution == resolution {
			return display, nil
		}
	}

	return DisplayInfo{diagonal, resolution}, errors.New("failed to match cros_healthd output to a known display")
}
