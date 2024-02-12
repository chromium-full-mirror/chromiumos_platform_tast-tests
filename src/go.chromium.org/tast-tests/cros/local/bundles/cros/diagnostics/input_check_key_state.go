// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputCheckKeyState,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Input page shows expected key states when keys are pressed and released",
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
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
	})
}

func InputCheckKeyState(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Open keyboard tester.
	if err := da.OpenKeyboardTester(ctx, tconn); err != nil {
		s.Fatal("Could not open keyboard tester: ", err)
	}

	ui := uiauto.New(tconn)
	verifyKeyState := func(keyName string) action.Action {
		actionName := "verify " + keyName + " key states right after pressing and releasing the key"
		return uiauto.NamedAction(actionName,
			uiauto.Combine(actionName,
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyNotPressed).First()),
				kb.AccelPressAction(keyName),
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyPressed).First()),
				kb.AccelReleaseAction(keyName),
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyTested).First()),
			))
	}

	if err := uiauto.Combine("verify the specific keys' states after pressing and releasing the key",
		verifyKeyState("backspace"),
		verifyKeyState("tab"),
		verifyKeyState("shift"),
		verifyKeyState("q"),
		verifyKeyState("1"),
	)(ctx); err != nil {
		s.Fatal("Failed to check key states: ", err)
	}
}
