// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"strings"

	"go.chromium.org/tast/core/errors"
)

// SupplyInfo holds the parsed power_supply_info information.
// The Battery field may be nil if the device is not present in the input.
type SupplyInfo struct {
	LinePower map[string]string
	Battery   map[string]string
}

// requiredKeys defines the complete, mandatory schema for each device type.
// The parser will fail if any of these keys are missing, or if any unexpected
// keys are present.
var requiredKeys = map[string][]string{
	"Line Power": {
		"path",
		"online",
		"type",
		"enum type",
		"voltage (V)",
		"current (A)",
		"max voltage (V)",
		"max current (A)",
		"active source",
		"available sources",
		"supports dual-role",
	},
	"Battery": {
		"path",
		"vendor",
		"model name",
		"state",
		"voltage (V)",
		"energy (Wh)",
		"energy rate (W)",
		"current (A)",
		"charge (Ah)",
		"full charge (Ah)",
		"full charge design (Ah)",
		"percentage",
		"display percentage",
		"technology",
	},
}

// ParseSupplyInfo parses the input string from power_supply_info command.
// It requires a "Line Power" device and allows for an optional "Battery" device.
// It strictly validates the key schema for any devices found.
//
// Example input:
//
//	Device: Line Power
//	  path:                    /sys/class/power_supply/...
//	  online:                  yes
//	  ...
//	Device: Battery
//	  path:                    /sys/class/power_supply/BAT0
//	  vendor:                  ...
//	  model name:              ...
//	  state:                   Fully charged
//	  ...
func ParseSupplyInfo(input string) (*SupplyInfo, error) {
	info := &SupplyInfo{}
	foundLinePower := false
	foundBattery := false

	// Filter out any empty strings that result from splitting on "Device: ".
	var sections []string
	for _, s := range strings.Split(input, "Device: ") {
		if strings.TrimSpace(s) != "" {
			sections = append(sections, s)
		}
	}

	// Require a "Line Power" device and allow for an optional "Battery" device,
	// so allow exactly 1 or 2 device sections.
	if len(sections) < 1 || len(sections) > 2 {
		return nil, errors.Errorf("invalid input format: expected 1 or 2 devices, but found %d", len(sections))
	}

	for _, section := range sections {
		deviceName, data, err := parseDeviceSection(section)
		if err != nil {
			return nil, err
		}

		switch deviceName {
		case "Line Power":
			if foundLinePower {
				return nil, errors.New("invalid input format: found duplicate 'Line Power' device")
			}
			info.LinePower = data
			foundLinePower = true
		case "Battery":
			if foundBattery {
				return nil, errors.New("invalid input format: found duplicate 'Battery' device")
			}
			info.Battery = data
			foundBattery = true
		default:
			return nil, errors.Errorf("invalid input format: found unexpected device: %q", deviceName)
		}
	}

	// "Line Power" is mandatory.
	if !foundLinePower {
		return nil, errors.New("invalid input format: missing required 'Line Power' device")
	}

	if err := validateKeys("Line Power", info.LinePower, requiredKeys["Line Power"]); err != nil {
		return nil, err
	}

	// "Battery" is optional, so only validate it if it was found.
	if foundBattery {
		if err := validateKeys("Battery", info.Battery, requiredKeys["Battery"]); err != nil {
			return nil, err
		}
	}

	return info, nil
}

// parseDeviceSection parses a single device's information and returns its name
// and data map.
//
// Example input:
//
//	Line Power
//	  path:                    /sys/class/power_supply/...
//	  online:                  yes
//	  type:                    USB_PD_DRP
//	  enum type:               AC
//	  voltage (V):             20
//	  current (A):             3.25
//	  max voltage (V):         20
//	  max current (A):         3.25
//	  active source:           ...
//	  available sources:       ... [/]
//	  supports dual-role:      yes
func parseDeviceSection(section string) (string, map[string]string, error) {
	lines := strings.Split(strings.TrimSpace(section), "\n")
	if len(lines) == 0 {
		return "", nil, errors.New("invalid device section: section is empty")
	}

	deviceName := strings.TrimSpace(lines[0])
	data := make(map[string]string)

	for _, line := range lines[1:] {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			data[key] = value
		}
	}
	return deviceName, data, nil
}

// validateKeys performs a two-way check against a schema.
func validateKeys(deviceName string, data map[string]string, required []string) error {
	// Create a set of required keys for efficient lookup.
	requiredSet := make(map[string]struct{}, len(required))
	for _, key := range required {
		requiredSet[key] = struct{}{}
	}

	// Check for missing keys.
	for _, key := range required {
		if _, ok := data[key]; !ok {
			return errors.Errorf("device %q is missing required key: %q", deviceName, key)
		}
	}

	// Check for unexpected keys.
	for key := range data {
		if _, ok := requiredSet[key]; !ok {
			return errors.Errorf("device %q has unexpected key: %q", deviceName, key)
		}
	}

	return nil
}
