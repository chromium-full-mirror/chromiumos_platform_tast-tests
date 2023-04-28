// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/network/netconfig"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowRoamingPolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the carrier name on connected esim",
		Contacts: []string{
			"nikhilcn@google.com",
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1226026",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func AllowRoamingPolicy(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	// Resets chrome and cleans up any pre-existing policies.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to reset chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)

	cellularONC := &policy.ONCCellular{
		AllowRoaming: false,
	}

	deviceProfileServiceGUID := "Cellular-Device-Policy"
	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			NetworkConfigurations: []*policy.ONCNetworkConfiguration{
				{
					GUID:     deviceProfileServiceGUID,
					Name:     "CellularDevicePolicyName",
					Type:     "Cellular",
					Cellular: cellularONC,
				},
			},
		},
	}

	// Apply Global Network Configuration.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
	}

	_, err = cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create connected cellular.Helper (precondition): ", err)
	}

	networkName, err := cellular.GetCellularNetwork(ctx)
	if err != nil {
		s.Fatal("Failed to get a cellular network: ", err)
	}

	app, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, networkName, netconfig.Cellular)
	if err != nil {
		s.Fatal("Failed to open network detail page: ", networkName)
	}
	defer app.Close(ctx)

	ui := uiauto.New(tconn)

	if err := ui.CheckRestriction(ossettings.RoamingToggle, restriction.Disabled)(ctx); err != nil {
		s.Fatal("Roaming toggle is not disabled: ", err)
	}
}
