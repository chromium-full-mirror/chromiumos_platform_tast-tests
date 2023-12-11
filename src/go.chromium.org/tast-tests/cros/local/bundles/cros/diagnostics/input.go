// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Input,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Can successfully navigate to the Input page",
		// ChromeOS > Software > System Services > Serviceability > Diagnostics
		BugComponent: "b:1131925",
		Contacts: []string{
			"cros-peripherals@google.com",
			"dpad@google.com",
			"ashleydp@google.com",
			"zentaro@google.com",
		},
		Fixture:      "diagnosticsPrepForInputDiagnostics",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

// Input verifies that the Input page can be navigated to.
func Input(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	// Since virtual keyboard with BUS_USB (0x03) doesn't work yet, use BUS_I2C (0x18).
	// See https://crrev.com/c/1407138 for more discussion.
	vkb, err := input.VirtualKeyboardWithBusType(ctx, 0x18)
	if err != nil {
		s.Fatal("Failed to create a virtual keyboard: ", err)
	}
	defer vkb.Close(ctx)

	if err := da.OpenInputPage(ctx, tconn); err != nil {
		s.Fatal("Could not click the menu button: ", err)
	}

	// Find the Input navigation item and the keyboard list heading.
	ui := uiauto.New(tconn)
	inputTab := da.DxKeyboardTab.Ancestor(da.DxRootNode)
	keyboardListHeading := da.DxKeyboardHeading.Ancestor(da.DxRootNode)
	if err := uiauto.Combine("find the keyboard list heading",
		ui.WaitUntilExists(inputTab),
		ui.WaitUntilExists(keyboardListHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to find the keyboard list heading: ", err)
	}
}
