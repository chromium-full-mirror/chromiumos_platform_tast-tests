// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputKeyboardShortcutClosesTester,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Keyboard input tester window closes with keyboard shortcut",
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

func InputKeyboardShortcutClosesTester(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := da.OpenKeyboardTester(ctx, tconn); err != nil {
		s.Fatal("Could not open keyboard tester: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Verify Alt+Esc keyboard shortcut closes keyboard tester",
		ui.WaitUntilExists(da.KeyNodeFinder("a", da.KeyNotPressed).First()),
		kb.AccelAction("a"),
		ui.WaitUntilExists(da.KeyNodeFinder("a", da.KeyTested).First()),
		// Closes keyboard tester window
		kb.AccelAction("alt+esc"),
		ui.WaitUntilGone(da.KeyNodeFinder("a", da.KeyTested).First()),
	)(ctx); err != nil {
		s.Error("Failed to check key states: ", err)
	}
}
