// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputKeyboardConnectAndDisconnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Connect a virtual keyboard on the diagnostics input page and then disconnect it",
		// ChromeOS > Software > System Services > Serviceability > Diagnostics
		BugComponent: "b:1131925",
		Contacts: []string{
			"cros-peripherals@google.com",
			"dpad@google.com",
			"jeff.lin@cienet.com",
			"xliu@cienet.com",
			"ashleydp@google.com",
		},
		Fixture:      "diagnosticsPrepForInputDiagnostics",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
	})
}

func InputKeyboardConnectAndDisconnect(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	// Since virtual keyboard BUS_USB (0x03) doesn't work yet, use BUS_I2C (0x18).
	// See https://crrev.com/c/1407138 for more discussion.
	vkb, err := input.VirtualKeyboardWithBusType(ctx, 0x18)
	if err != nil {
		s.Fatal("Failed to create a virtual keyboard: ", err)
	}
	defer vkb.Close(ctx)

	if err := da.OpenInputPage(ctx, tconn); err != nil {
		s.Fatal("Could open the input page: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("check no virtual keyboard exists in input device list",
		ui.Gone(da.DxVirtualKeyboardHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to check virtual keyboard: ", err)
	}

	disconnectKeyboard := func() action.Action {
		return func(ctx context.Context) error {
			return vkb.Close(ctx)
		}
	}

	if err := uiauto.Combine("verify virtual keyboard appears and disappears in the device list",
		ui.WaitUntilExists(da.DxVirtualKeyboardHeading),
		disconnectKeyboard(),
		ui.WaitUntilGone(da.DxVirtualKeyboardHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to execute keyboard test: ", err)
	}
}
