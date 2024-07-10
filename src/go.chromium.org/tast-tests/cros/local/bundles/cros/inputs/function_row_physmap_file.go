// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// minimum number of scan codes that are expected in the function_row_physmap file
	minimumNumExpectedCodes = 10

	// path to the function_row_physmap file in the device's sys path
	functionRowPhysmapDevicePath = "device/function_row_physmap"
)

// Scan codes taken from the action_keymaps array:
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/coreboot/src/acpi/acpigen_ps2_keybd.c
var validVivaldiScanCodes = [...]uint8{
	0xea, /* KEY_BACK */
	0xe9, /* KEY_FORWARD */
	0xe7, /* KEY_REFRESH */
	0x91, /* KEY_FULL_SCREEN */
	0x92, /* KEY_SCALE */
	0xa0, /* KEY_MUTE */
	0xae, /* KEY_VOLUMEDOWN */
	0xb0, /* KEY_VOLUMEUP */
	0x9a, /* KEY_PLAYPAUSE */
	0x99, /* KEY_NEXTSONG */
	0x90, /* KEY_PREVIOUSSONG */
	0x93, /* KEY_SYSRQ */
	0x94, /* KEY_BRIGHTNESSDOWN */
	0x95, /* KEY_BRIGHTNESSUP */
	0x97, /* KEY_KBDILLUMDOWN */
	0x98, /* KEY_KBDILLUMUP */
	0x96, /* KEY_PRIVACY_SCREEN_TOGGLE */
	0x9b, /* KEY_MICMUTE */
	0x9e, /* KEY_KBDILLUMTOGGLE */
	0xdd, /* KEY_CONTROLPANEL */
	0xa7, /* KEY_DICTATE */
	0xa9, /* KEY_ACCESSIBILITY */
	0xa8, /* KEY_DO_NOT_DISTURB */
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FunctionRowPhysmapFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validate the contents of the function_row_physmap file",
		Contacts: []string{
			"chromeos-tango@google.com",
			"joeyholtzman@google.com", // Test author
		},
		BugComponent: "b:167212", // ChromeOS > Platform > baseOS > Input
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Keyboard()),
		SoftwareDeps: []string{
			"amd64", // TODO(b/351726739): Test ARM64 keyboards as well
		},
	})
}

func FunctionRowPhysmapFile(ctx context.Context, s *testing.State) {
	// Find the full /sys/devices path of the physical keyboard,
	// e.g. "/sys/devices/platform/i8042/serio0/input/input2".
	foundPhysicalKeyboard, keyboardSysPath, err := input.FindPhysicalKeyboardSysPath(ctx)
	if err != nil {
		s.Fatal("Failed to query system for physical keyboard: ", err)
	}
	if !foundPhysicalKeyboard {
		s.Fatal("No physical keyboard connected")
	}

	// Read the contents of the function_row_physmap file for the keyboard.
	functionRowPhysmapFile := path.Join(keyboardSysPath, functionRowPhysmapDevicePath)
	fileBytes, err := os.ReadFile(functionRowPhysmapFile)
	if err != nil {
		// This file will not be present for non-Vivaldi devices.
		// TODO(b/351726739): Don't run test for non-Vivaldi devices.
		s.Fatalf("Failed to read function_row_physmap file at %s: %v", functionRowPhysmapFile, err)
	}

	// Convert the file contents to a string to later be parsed into hex numbers.
	fileContents := strings.TrimSuffix(string(fileBytes), "\n")
	s.Log("Contents of the function_row_physmap file: ", fileContents)

	// Every scan code is separated by a space in the function_row_physmap file.
	scanCodeStrs := strings.Split(fileContents, " ")

	if len(scanCodeStrs) < minimumNumExpectedCodes {
		s.Errorf("Not enough scan codes are present (have %d, need at least %d)", len(scanCodeStrs), minimumNumExpectedCodes)
	}

	nonZero := false
	for _, scanCodeStr := range scanCodeStrs {
		// Decode each scan code into a 1-byte hex number.
		scanCode64, err := strconv.ParseUint(scanCodeStr, 16, 8)
		if err != nil {
			s.Errorf("Failed to parse scan code %s: %v", scanCodeStr, err)
			continue
		}

		scanCode := uint8(scanCode64)
		if scanCode != 0 {
			// Record that not all scan codes are zero.
			nonZero = true
		}

		if !isValidVivaldiScanCode(scanCode) {
			s.Errorf("'%X' is not a valid scan code", scanCode)
		}

	}

	if !nonZero {
		// The function_row_physmap file can contain all zeros when there is a failure in parsing
		// the HID report for the device.
		s.Fatal("function_row_physmap file contains all zeros")
	}
}

// isValidVivaldiScanCode checks if the provided scan code is in the array of known Vivaldi scan codes.
func isValidVivaldiScanCode(scanCode uint8) bool {
	for _, validScanCode := range validVivaldiScanCodes {
		if scanCode == validScanCode {
			return true
		}
	}

	return false
}
