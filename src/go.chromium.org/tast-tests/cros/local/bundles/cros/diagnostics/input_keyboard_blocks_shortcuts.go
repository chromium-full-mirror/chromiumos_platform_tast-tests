// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	la "go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputKeyboardBlocksShortcuts,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Keyboard input tester blocks all shortcuts only when open",
		// ChromeOS > Software > System Services > Serviceability > Diagnostics
		BugComponent: "b:1131925",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"dpad@google.com",
		},
		Fixture:      "diagnosticsPrepForInputDiagnostics",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard(), hwdep.NoSplitModifierKeyboard()),
	})
}

func InputKeyboardBlocksShortcuts(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	kb, err := input.KeyboardTabletModeForceDisabled(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := da.OpenKeyboardTester(ctx, tconn); err != nil {
		s.Fatal("Could not open keyboard tester: ", err)
	}

	launcher := nodewith.ClassName(la.ExpandedItemsClass).Visible().First()
	diagnosticsApp := da.DxRootNode
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Verify Shortcuts are blocked in keyboard tester window",
		ui.WaitUntilExists(da.KeyNodeFinder("a", da.KeyNotPressed).First()),
		// Control + w should not close the window
		kb.AccelAction("ctrl+w"),
		ui.WaitUntilExists(diagnosticsApp),
		// Try to open launcher and verify it does not open
		kb.AccelAction("search"),
		ui.Gone(launcher),
		// Closes keyboard tester window
		kb.AccelAction("alt+esc"),
		// Verify launcher can be launched now
		kb.AccelAction("search"),
		ui.WaitUntilExists(launcher),
		kb.AccelAction("search"),
		ui.WaitUntilGone(launcher),
		// Verify ctrl + w will close the diagnostics window
		kb.AccelAction("ctrl+w"),
		ui.WaitUntilGone(diagnosticsApp),
	)(ctx); err != nil {
		s.Error("Failed to check key states: ", err)
	}
}
