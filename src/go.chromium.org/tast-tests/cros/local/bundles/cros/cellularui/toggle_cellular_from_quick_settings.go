// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ToggleCellularFromQuickSettings,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Checks that Cellular can be enabled and disabled from within the Quick Settings",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic"},
		Fixture: "cellularEnforceConnectionLocal",
	})
}

// ToggleCellularFromQuickSettings tests that a user can successfully toggle
// the Cellular state using the Quick Settings.
func ToggleCellularFromQuickSettings(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create new chrome instance: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Disable auto-connect so that enabling cellular data will not automatically trigger a connection, reducing instability caused by connection attempts.
	cleanup, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, false)
	if err != nil {
		s.Fatal("Failed to initialize autoconnect to false: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

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

	mobileDataContainer := nodewith.Role(role.GenericContainer).NameContaining("Mobile data is turned on")

	// Quick settings does not use a button with a checked state, so look for the name.
	if err := uiauto.Combine("Wait until cellular is enabled in UI and not inhibited",
		ui.WaitUntilExists(mobileDataToggle.Ancestor(mobileDataContainer)),
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
		if state {
			mobileDataContainer = nodewith.Role(role.GenericContainer).NameContaining("Mobile data is turned on")
		} else {
			mobileDataContainer = nodewith.Role(role.GenericContainer).NameContaining("Mobile data is turned off")
		}
		if err := uiauto.Combine("Wait until cellular is in expected state in UI and not inhibited",
			ui.WaitUntilExists(mobileDataToggle.Ancestor(mobileDataContainer)),
			ui.WaitUntilEnabled(mobileDataToggle),
		)(ctx); err != nil {
			s.Fatal("Failed: ", err)
		}
		state = !state
	}
}
