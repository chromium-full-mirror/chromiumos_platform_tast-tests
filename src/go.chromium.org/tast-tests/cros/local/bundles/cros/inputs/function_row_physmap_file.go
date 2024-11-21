// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// minimum number of scan codes that are expected in the function_row_physmap file
	minimumNumExpectedCodes = 10

	// path to the function_row_physmap file in the device's sys path
	functionRowPhysmapDevicePath = "device/function_row_physmap"

	// path to the ARM keyboard's row and columns properties in its device tree
	armKeyboardRowsPath    = "device/of_node/keypad,num-rows"
	armKeyboardColumnsPath = "device/of_node/keypad,num-columns"
)

// Scan codes taken from the action_keymaps array:
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/coreboot/src/acpi/acpigen_ps2_keybd.c
var validAMDScanCodes = [...]uint32{
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

// keyboardInfo holds information necessary for validating an ARM keyboard's
// function_row_physmap file.
type keyboardInfo struct {
	numColumns uint32
	numRows    uint32
	shift      uint32
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FunctionRowPhysmapFile,
		Desc: "Validate the contents of the function_row_physmap file",
		Contacts: []string{
			"chromeos-tango@google.com",
		},
		BugComponent: "b:167212", // ChromeOS > Platform > baseOS > Input
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.CustomTopRowKeyboard()),
		SoftwareDeps: []string{
			"custom_top_row_keyboard",
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

	keyboardInfo := &keyboardInfo{}
	isArmDevice := runtime.GOARCH == "arm" || runtime.GOARCH == "arm64"
	if isArmDevice {
		// ARM devices use the keyboard's rows and columns to encode the scancodes, so
		// this info needs to be retrieved from the keyboard's device tree.
		err = populateKeyboardInfo(keyboardInfo, keyboardSysPath)
		if err != nil {
			s.Fatal("Failed to populate keyboard info: ", err)
		}
	}

	nonZero := false
	for _, scanCodeStr := range scanCodeStrs {
		// Decode each scan code into a 4-byte hex number.
		scanCode64, err := strconv.ParseUint(scanCodeStr, 16, 32)
		if err != nil {
			s.Errorf("Failed to parse scan code %s: %v", scanCodeStr, err)
			continue
		}

		scanCode := uint32(scanCode64)
		if scanCode != 0 {
			// Record that not all scan codes are zero.
			nonZero = true
		}

		if isArmDevice {
			if !isValidARMScanCode(scanCode, keyboardInfo) {
				s.Errorf("'%X' is not a valid scan code", scanCode)
			}
		} else {
			if !isValidAMDScanCode(scanCode) {
				s.Errorf("'%X' is not a valid scan code", scanCode)
			}
		}

	}

	if !nonZero {
		// The function_row_physmap file can contain all zeros when there is a failure in parsing
		// the HID report for the device.
		s.Fatal("function_row_physmap file contains all zeros")
	}
}

// populateKeyboardInfo reads the keyboard's rows and columns properties from
// its device tree and populates the provided keyboardInfo.
func populateKeyboardInfo(keyboardInfo *keyboardInfo, keyboardSysPath string) error {
	// Read keyboard rows.
	keyboardRowsFile := path.Join(keyboardSysPath, armKeyboardRowsPath)
	numRowsBytes, err := os.ReadFile(keyboardRowsFile)
	if err != nil {
		return errors.Wrap(err, "failed to read keyboard's num-rows property")
	}
	keyboardInfo.numRows = binary.BigEndian.Uint32(numRowsBytes)

	// Read keyboard columns.
	keyboardColumnsFile := path.Join(keyboardSysPath, armKeyboardColumnsPath)
	numColumnsBytes, err := os.ReadFile(keyboardColumnsFile)
	if err != nil {
		return errors.Wrap(err, "failed to read keyboard's num-columns property")
	}
	keyboardInfo.numColumns = binary.BigEndian.Uint32(numColumnsBytes)

	// Keyboard shift is log2 of the keyboard's columns.
	keyboardInfo.shift = uint32(math.Log2(float64(keyboardInfo.numColumns)))

	return nil
}

// isValidARMScanCode decodes the provided ARM-encoded scan code into its row
// and column and validates that both the row and column are less than the
// keyboard's total number of rows and columns, respectively.
//
// As seen in the MATRIX_SCAN_CODE definition at the link below, each scan code
// is encoded as follows:
// scanCode = (row << shift) + col
//
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/kernel/upstream/include/linux/input/matrix_keypad.h
func isValidARMScanCode(scanCode uint32, keyboardInfo *keyboardInfo) bool {
	row := scanCode >> keyboardInfo.shift
	col := scanCode & ((1 << keyboardInfo.shift) - 1)
	return row < keyboardInfo.numRows && col < keyboardInfo.numColumns
}

// isValidAMDScanCode checks if the provided scan code is in the array of known
// AMD Vivaldi scan codes.
func isValidAMDScanCode(scanCode uint32) bool {
	for _, validScanCode := range validAMDScanCodes {
		if scanCode == validScanCode {
			return true
		}
	}

	return false
}
