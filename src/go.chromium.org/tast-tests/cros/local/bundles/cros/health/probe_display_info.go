// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"math"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeDisplayInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for display info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"kerker@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		// TODO(b/326832237): Promote to critical.
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"diagnostics"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "crosHealthdRunning",
	})
}

type displayInfo struct {
	EDP embeddedDisplayInfo    `json:"embedded_display"`
	DP  *[]externalDisplayInfo `json:"external_displays"`
}

// Privacy screen state is not verified in current test since we need to get the info from modetest.
type embeddedDisplayInfo struct {
	PrivacyScreenEnabled   bool              `json:"privacy_screen_enabled"`
	PrivacyScreenSupported bool              `json:"privacy_screen_supported"`
	DisplayWidth           *jsontypes.Uint32 `json:"display_width"`
	DisplayHeight          *jsontypes.Uint32 `json:"display_height"`
	ResolutionHorizontal   *jsontypes.Uint32 `json:"resolution_horizontal"`
	ResolutionVertical     *jsontypes.Uint32 `json:"resolution_vertical"`
	RefreshRate            *float64          `json:"refresh_rate"`
	Manufacturer           string            `json:"manufacturer"`
	ModelID                *uint16           `json:"model_id"`
	SerialNumber           *jsontypes.Uint32 `json:"serial_number"`
	ManufactureWeek        *uint8            `json:"manufacture_week"`
	ManufactureYear        *uint16           `json:"manufacture_year"`
	EdidVersion            string            `json:"edid_version"`
	InputType              string            `json:"input_type"`
	DisplayName            string            `json:"display_name"`
}

type externalDisplayInfo struct {
	DisplayWidth         *jsontypes.Uint32 `json:"display_width"`
	DisplayHeight        *jsontypes.Uint32 `json:"display_height"`
	ResolutionHorizontal *jsontypes.Uint32 `json:"resolution_horizontal"`
	ResolutionVertical   *jsontypes.Uint32 `json:"resolution_vertical"`
	RefreshRate          *float64          `json:"refresh_rate"`
	Manufacturer         string            `json:"manufacturer"`
	ModelID              *uint16           `json:"model_id"`
	SerialNumber         *jsontypes.Uint32 `json:"serial_number"`
	ManufactureWeek      *uint8            `json:"manufacture_week"`
	ManufactureYear      *uint16           `json:"manufacture_year"`
	EdidVersion          string            `json:"edid_version"`
	InputType            string            `json:"input_type"`
	DisplayName          string            `json:"display_name"`
}

func compareUintPointer[T uint8 | uint16 | uint32](got *T, want T, field string) error {
	if got == nil {
		if want != 0 {
			return errors.Errorf("failed. %s doesn't match: got nil; want %v", field, want)
		}
	} else if want != *got {
		return errors.Errorf("failed. %s doesn't match: got %v; want %v", field, *got, want)
	}

	return nil
}

var maximumImageSizeRegexp = regexp.MustCompile(`Maximum image size: (.*) cm x (.*) cm`)

func verifyEmbeddedDisplaySize(ctx context.Context, edp *embeddedDisplayInfo, edid string) error {
	match := maximumImageSizeRegexp.FindStringSubmatch(edid)
	if match == nil {
		return errors.New("failed to parse edid to get size information")
	}

	if width, err := strconv.ParseUint(match[1], 10, 32); err != nil {
		return err
	} else if err := compareUintPointer((*uint32)(edp.DisplayWidth), uint32(width)* /* cm to mm */ 10, "DisplayWidth"); err != nil {
		return err
	}

	if height, err := strconv.ParseUint(match[2], 10, 32); err != nil {
		return err
	} else if err := compareUintPointer((*uint32)(edp.DisplayHeight), uint32(height)* /*cm to mm */ 10, "DisplayHeight"); err != nil {
		return err
	}

	return nil
}

var detailedTimingDescriptionRegexp = regexp.MustCompile(`DTD \d:\s*(\d*)x(\d*)\s*(\d*\.\d*)\s*Hz.*`)

func verifyEmbeddedDisplayResolutionAndRefreshRate(ctx context.Context, edp *embeddedDisplayInfo, edid string) error {
	matches := detailedTimingDescriptionRegexp.FindAllStringSubmatch(edid /* there should not be more than 10 preference */, 10)
	if matches == nil {
		return errors.New("failed to parse the edid info to get the detailed timing description")
	}

	for _, match := range matches {
		// Since there could be several display preference data, and only one can match. So we ignore all errors here.
		// For debug, we just simply check the edid output in the beginning of this test.
		allMatch := true
		horizontal, _ := strconv.ParseUint(match[1], 10, 32)
		vertical, _ := strconv.ParseUint(match[2], 10, 32)
		refreshRate, _ := strconv.ParseFloat(match[3], 64)

		if err := compareUintPointer((*uint32)(edp.ResolutionHorizontal), uint32(horizontal), "ResolutionHorizontal"); err != nil {
			allMatch = false
		}
		if err := compareUintPointer((*uint32)(edp.ResolutionVertical), uint32(vertical), "ResolutionVertical"); err != nil {
			allMatch = false
		}
		if math.Abs(refreshRate-*edp.RefreshRate) > 1 {
			allMatch = false
		}

		if allMatch {
			return nil
		}
	}

	return errors.Errorf("failed to find a match of display preference, parse result is: %s", matches)

}

var manufacturerRegexp = regexp.MustCompile(`Manufacturer: (.*)`)
var modelIDRegexp = regexp.MustCompile(`Model: (.*)`)
var serialNumberRegexp = regexp.MustCompile(`Serial Number: (.*)`)

func verifyEmbeddedDisplayIdentifier(ctx context.Context, edp *embeddedDisplayInfo, edidInfo string) error {
	manufacturer := ""
	if manufacturerMatch := manufacturerRegexp.FindStringSubmatch(edidInfo); manufacturerMatch != nil {
		manufacturer = manufacturerMatch[1]
	}
	if manufacturer != edp.Manufacturer {
		return errors.Errorf("failed. Manufacturer doesn't match: got %v; want %v", edp.Manufacturer, manufacturer)
	}

	modelIDRaw := ""
	if modelIDMatch := modelIDRegexp.FindStringSubmatch(edidInfo); modelIDMatch != nil {
		modelIDRaw = modelIDMatch[1]
	}
	if modelIDRaw == "" && edp.ModelID != nil {
		return errors.New("there is no ModelID info, but cros_healthd report it")
	} else if modelIDRaw != "" {
		if modelID, err := strconv.ParseUint(modelIDRaw, 10, 16); err != nil {
			return err
		} else if err := compareUintPointer(edp.ModelID, uint16(modelID), "ModelID"); err != nil {
			return err
		}
	}

	serialNumberRaw := ""
	if serialNumberMatch := serialNumberRegexp.FindStringSubmatch(edidInfo); serialNumberMatch != nil {
		serialNumberRaw = serialNumberMatch[1]
	}
	if serialNumberRaw == "" && edp.SerialNumber != nil {
		return errors.New("there is no SerialNumber info, but cros_healthd report it")
	} else if serialNumberRaw != "" {
		if serialNumber, err := strconv.ParseUint(serialNumberRaw, 10, 32); err != nil {
			return err
		} else if err := compareUintPointer((*uint32)(edp.SerialNumber), uint32(serialNumber), "SerialNumber"); err != nil {
			return err
		}
	}

	return nil
}

var manufactureYearRegexp = regexp.MustCompile(`Made in:.*([0-9]{4})`)
var manufactureWeekRegexp = regexp.MustCompile(`Made in: week (.*) of [0-9]{4}`)

func verifyEmbeddedDisplayManufactureDate(ctx context.Context, edp *embeddedDisplayInfo, edidInfo string) error {
	manufactureYearRaw := ""
	if manufactureYearMatch := manufactureYearRegexp.FindStringSubmatch(edidInfo); manufactureYearMatch != nil {
		manufactureYearRaw = manufactureYearMatch[1]
	}
	if manufactureYearRaw == "" && edp.ManufactureYear != nil {
		return errors.New("there is no ManufactureYear info, but cros_healthd report it")
	} else if manufactureYearRaw != "" {
		if manufactureYear, err := strconv.ParseUint(manufactureYearRaw, 10, 16); err != nil {
			return err
		} else if err := compareUintPointer(edp.ManufactureYear, uint16(manufactureYear), "ManufactureYear"); err != nil {
			return err
		}
	}

	// Reference: VESA E-EDID Standard Release A2 - Table 3.8
	// https://glenwing.github.io/docs/VESA-EEDID-A2.pdf
	const maxManufactureWeek = 54
	manufactureWeekRaw := ""
	if manufactureWeekMatch := manufactureWeekRegexp.FindStringSubmatch(edidInfo); manufactureWeekMatch != nil {
		manufactureWeekRaw = manufactureWeekMatch[1]
	}
	if manufactureWeekRaw == "" && edp.ManufactureWeek != nil {
		return errors.New("there is no ManufactureWeek info, but cros_healthd report it")
	} else if manufactureWeekRaw != "" {
		if manufactureWeek, err := strconv.ParseUint(manufactureWeekRaw, 10, 8); err != nil {
			return err
		} else if manufactureWeek == 0 || manufactureWeek > maxManufactureWeek {
			if edp.ManufactureWeek != nil {
				return errors.Errorf("ManufactureWeek value %v is out of range [1, %v], but cros_healthd report it: got %v",
					manufactureWeek, maxManufactureWeek, edp.ManufactureWeek)
			}
		} else if err := compareUintPointer(edp.ManufactureWeek, uint8(manufactureWeek), "ManufactureWeek"); err != nil {
			return err
		}
	}

	return nil
}

var inputTypeRegexp = regexp.MustCompile(`Basic Display Parameters & Features:\n {4}(.*) display`)
var displayNameRegexp = regexp.MustCompile(`Display Product Name: '(.*)'`)

func verifyEmbeddedDisplayProperty(ctx context.Context, edp *embeddedDisplayInfo, edidInfo string) error {
	inputType := ""
	if inputTypeMatch := inputTypeRegexp.FindStringSubmatch(edidInfo); inputTypeMatch != nil {
		inputType = inputTypeMatch[1]
	}
	if inputType != edp.InputType {
		return errors.Errorf("failed. InputType doesn't match: got %v; want %v", edp.InputType, inputType)
	}

	displayName := ""
	if displayNameMatch := displayNameRegexp.FindStringSubmatch(edidInfo); displayNameMatch != nil {
		displayName = displayNameMatch[1]
	}
	if displayName != edp.DisplayName {
		return errors.Errorf("failed. DisplayName doesn't match: got %v; want %v", edp.DisplayName, displayName)
	}

	return nil
}

var edidVersionRegexp = regexp.MustCompile(`EDID Structure Version & Revision: (.*)`)

func verifyEmbeddedDisplayEdidInfo(ctx context.Context, edp *embeddedDisplayInfo, edid string) error {
	if err := verifyEmbeddedDisplayIdentifier(ctx, edp, edid); err != nil {
		return errors.Wrap(err, "failed to verify embedded display identifier")
	}
	if err := verifyEmbeddedDisplayManufactureDate(ctx, edp, edid); err != nil {
		return errors.Wrap(err, "failed to verify embedded display manufacture date")
	}
	if err := verifyEmbeddedDisplayProperty(ctx, edp, edid); err != nil {
		return errors.Wrap(err, "failed to verify embedded display property")
	}

	edidVersion := ""
	if edidVersionMatch := edidVersionRegexp.FindStringSubmatch(edid); edidVersionMatch != nil {
		edidVersion = edidVersionMatch[1]
	}
	if edidVersion != edp.EdidVersion {
		return errors.Errorf("failed. EdidVersion doesn't match: got %v; want %v", edp.EdidVersion, edidVersion)
	}

	return nil
}

func verifyEmbeddedDisplayInfo(ctx context.Context, edp *embeddedDisplayInfo, edid string) error {
	if err := verifyEmbeddedDisplaySize(ctx, edp, edid); err != nil {
		return err
	}
	if err := verifyEmbeddedDisplayResolutionAndRefreshRate(ctx, edp, edid); err != nil {
		return err
	}
	if err := verifyEmbeddedDisplayEdidInfo(ctx, edp, edid); err != nil {
		return err
	}

	return nil
}

func ProbeDisplayInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryDisplay}
	var display displayInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &display); err != nil {
		s.Fatal("Failed to get display telemetry info: ", err)
	}

	b, err := testexec.CommandContext(ctx, "edid-decode", "/sys/class/drm/card0-eDP-1/edid").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get EDID information, err: ", err)
	}
	edid := string(b)
	s.Log("EDID information: ", edid)

	if err := verifyEmbeddedDisplayInfo(ctx, &display.EDP, edid); err != nil {
		s.Fatal("Failed to validate embedded display info, err: ", err)
	}
}
