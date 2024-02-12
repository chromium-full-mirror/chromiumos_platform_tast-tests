// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	da "go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InputCheckDefocusing,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Pressing and releasing keys won't affect key states when the input page isn't focused",
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

func InputCheckDefocusing(ctx context.Context, s *testing.State) {
	cr, tconn := s.FixtValue().(*utils.FixtureData).Cr, s.FixtValue().(*utils.FixtureData).Tconn

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Wait until diagnostics window is open and lock to left side of the screen.
	diagnosticsWindow, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, apps.Diagnostics.Name)
	if err != nil {
		s.Fatal("Failed to find diagnostics window: ", err)
	}
	ash.SetWindowStateAndWait(ctx, tconn, diagnosticsWindow.ID, ash.WindowStatePrimarySnapped)

	conn, err := cr.NewConn(ctx, "https://www.google.com")
	if err != nil {
		s.Fatal("Failed to create chrome: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	chromeWindow, err := ash.WaitForAnyWindowWithoutTitle(ctx, tconn, apps.Diagnostics.Name)
	if err != nil {
		s.Fatal("Failed to find browser window: ", err)
	}
	ash.SetWindowStateAndWait(ctx, tconn, chromeWindow.ID, ash.WindowStateSecondarySnapped)

	// Finds the browser window and shifts focus to it
	focusBrowserWindow := func(ctx context.Context) error {
		return chromeWindow.ActivateWindow(ctx, tconn)
	}

	// Finds the browser window and shifts focus to it
	focusDiagnosticsWindow := func(ctx context.Context) error {
		return diagnosticsWindow.ActivateWindow(ctx, tconn)
	}

	// Open keyboard tester.
	if err := da.OpenKeyboardTester(ctx, tconn); err != nil {
		s.Fatal("Could not open keyboard tester: ", err)
	}

	ui := uiauto.New(tconn)
	verifyKeyStateUnaffected := func(keyName string) action.Action {
		actionName := "verify " + keyName + " key states when input page isn't focused"
		return uiauto.NamedAction(actionName,
			uiauto.Combine(actionName,
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyNotPressed).First()),
				kb.AccelPressAction(keyName),
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyNotPressed).First()),
				kb.AccelReleaseAction(keyName),
				ui.WaitUntilExists(da.KeyNodeFinder(keyName, da.KeyNotPressed).First()),
			))
	}

	if err := uiauto.Combine("verify pressing and releasing key won't affect key states",
		// Pressing and releasing an inoccuous key and check it's shown as tested.
		kb.AccelAction("x"),
		ui.WaitUntilExists(da.KeyNodeFinder("x", da.KeyTested).First()),
		// Switch focus to a different window and check a pops up message when losing the focus.
		focusBrowserWindow,
		ui.WaitUntilExists(da.DxDefocusingMsg),
		// Pressing and releasing a few keys, each time checking keys are not reflected.
		verifyKeyStateUnaffected("shift"),
		verifyKeyStateUnaffected("1"),
		verifyKeyStateUnaffected("q"),
		// Switching focus back to the Diagnostics window and check pops up message is gone.
		focusDiagnosticsWindow,
		ui.WaitUntilGone(da.DxDefocusingMsg),
		// Checking an inoccuous key still shown as tested.
		ui.WaitUntilExists(da.KeyNodeFinder("x", da.KeyTested).First()),
		// Checking keys are still marked as not pressed.
		ui.WaitUntilExists(da.KeyNodeFinder("shift", da.KeyNotPressed).First()),
		ui.WaitUntilExists(da.KeyNodeFinder("1", da.KeyNotPressed).First()),
		ui.WaitUntilExists(da.KeyNodeFinder("q", da.KeyNotPressed).First()),
		// Pressing another key and check it is reflected in the diagram.
		kb.AccelAction("ctrl"),
		ui.WaitUntilExists(da.KeyNodeFinder("ctrl", da.KeyTested).First()),
	)(ctx); err != nil {
		s.Fatal("Failed to check key states: ", err)
	}
}
