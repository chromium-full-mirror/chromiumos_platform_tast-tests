// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowCellularPolicyNetworksOn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the policy that only allows managed cellular networks",
		Contacts: []string{
			"cros-connectivity@google.com",
			"nikhilcn@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolledAndTestSIM",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func AllowCellularPolicyNetworksOn(ctx context.Context, s *testing.State) {
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

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyCellularNetworks: true,
	}

	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			GlobalNetworkConfiguration: globalConfig,
			NetworkConfigurations:      []*policy.ONCNetworkConfiguration{},
		},
	}

	// Apply Global Network Configuration.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
	}

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	if err := helper.WaitForEnabledState(ctx, true); err != nil {
		s.Fatal("Failed to enable Cellular state: ", err)
	}

	app, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS Settings: ", err)
	}

	defer app.Close(ctx)

	_, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data settings: ", err)
	}

	ui := uiauto.New(tconn)

	if err := ui.CheckRestriction(ossettings.AddCellularButton, restriction.Disabled)(ctx); err != nil {
		s.Fatal("Add cellular button is not disabled: ", err)
	}

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Network view: ", err)
	}

	if err := ui.Gone(quicksettings.AddCellularButton)(ctx); err != nil {
		s.Fatal("Add cellular button is still present: ", err)
	}
}
