// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
	foundPhysicalKeyboard, _, err := input.FindPhysicalKeyboardSysPath(ctx)
	if err != nil {
		s.Fatal("Failed to query system for physical keyboard: ", err)
	}
	if !foundPhysicalKeyboard {
		s.Fatal("No physical keyboard connected")
	}

	// TODO(b/351726739): Ensure the function_row_physmap file exists at <keyboard location>/device/function_row_physmap.
	// TODO(b/351726739): Parse all scan codes in the file into hex numbers and validate at least 10 exist.
	// TODO(b/351726739): Ensure every scan code is a valid Vivaldi scan code and that all scan codes are not zero.
	// TODO(b/351726739): Skip running the test if the keyboard/device does not support Vivaldi.
}
