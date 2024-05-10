// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ToggleCellularFromQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Checks that Cellular can be enabled and disabled from within the Quick Settings",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		Fixture:      "cellularE2ELocal",
	})
}

// ToggleCellularFromQuickSettings tests that a user can successfully toggle
// the Cellular state using the Quick Settings.
func ToggleCellularFromQuickSettings(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create new chrome instance: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	ui := uiauto.New(tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Network view: ", err)
	}

	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Cellular: ", err)
	}

	mobileDataToggle, err := quicksettings.NetworkDetailedViewMobileDataToggle(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get mobile data toggle: ", err)
	}

	// Quick settings does not use a button with a checked state, so look for the name.
	if err := uiauto.Combine("Wait until cellular is enabled in UI and not inhibited",
		ui.WaitUntilExists(mobileDataToggle.NameContaining("Mobile data is turned on")),
		ui.WaitUntilEnabled(mobileDataToggle),
	)(ctx); err != nil {
		s.Fatal("Failed: ", err)
	}

	state := false
	const iterations = 5

	for i := 0; i < iterations; i++ {
		s.Logf("Toggling Cellular (iteration %d of %d)", i+1, iterations)

		if err := ui.LeftClick(mobileDataToggle)(ctx); err != nil {
			s.Fatal("Failed to click on cellular toggle button")
		}

		if err := helper.WaitForEnabledState(ctx, state); err != nil {
			s.Fatal("Failed to toggle Cellular state: ", err)
		}

		// Quick settings does not use a button with a checked state, so look for the name.
		var name string
		if state {
			name = "Mobile data is turned on"
		} else {
			name = "Mobile data is turned off"
		}
		if err := uiauto.Combine("Wait until cellular is in expected state in UI and not inhibited",
			ui.WaitUntilExists(mobileDataToggle.NameContaining(name)),
			ui.WaitUntilEnabled(mobileDataToggle),
		)(ctx); err != nil {
			s.Fatal("Failed: ", err)
		}
		state = !state
	}
}
