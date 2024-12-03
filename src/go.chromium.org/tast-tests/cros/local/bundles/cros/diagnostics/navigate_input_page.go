// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diagnostics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/diagnostics/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NavigateInputPage,
		Desc: "Can successfully navigate to the Input page",
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
		},
		Fixture: "diagnosticsPrepForInputDiagnostics",
		Attr: []string{"group:mainline", "informational",
			"group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard(), hwdep.NoSplitModifierKeyboard()),
	})
}

// NavigateInputPage verifies that the Input page can be navigated to.
func NavigateInputPage(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*utils.FixtureData).Tconn

	if err := diagnosticsapp.OpenInputPage(ctx, tconn); err != nil {
		s.Fatal("Could not click the menu button: ", err)
	}

	// Find the Input navigation item and the keyboard list heading.
	ui := uiauto.New(tconn)
	inputTab := diagnosticsapp.DxKeyboardTab.Ancestor(diagnosticsapp.DxRootNode)
	keyboardListHeading := diagnosticsapp.DxKeyboardHeading.Ancestor(diagnosticsapp.DxRootNode)
	if err := uiauto.Combine("find the keyboard list heading",
		ui.WaitUntilExists(inputTab),
		ui.WaitUntilExists(keyboardListHeading),
	)(ctx); err != nil {
		s.Fatal("Failed to find the keyboard list heading: ", err)
	}
}
