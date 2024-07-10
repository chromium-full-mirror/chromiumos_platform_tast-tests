// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"os"
	"path"
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

	// Every scan code is separated by a space in the function_row_physmap file.
	scanCodeStrs := strings.Split(fileContents, " ")

	if len(scanCodeStrs) < minimumNumExpectedCodes {
		s.Errorf("Not enough scan codes are present (have %d, need at least %d)", len(scanCodeStrs), minimumNumExpectedCodes)
	}

	// TODO(b/351726739): Ensure every scan code is a valid Vivaldi scan code and that all scan codes are not zero.
}
