// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           AllowRoamingPolicy,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests the carrier name on connected esim",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_sim_prod_esim"},
		Fixture: "cellularWithFakeDMSEnrolled",
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

	tconn, _ := cr.TestAPIConn(ctx)

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

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil || networkName == "" {
		s.Fatal("Failed to fetch network name by iccid, err: ", err)
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data sub page: ", err)
	}
	defer app.Close(ctx)

	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

	connectedCellularRow := nodewith.NameContaining(networkName + ", Connected").First()

	if err = ui.LeftClick(connectedCellularRow)(ctx); err != nil {
		s.Fatal("Failed to click on connected network row: ", err)
	}

	if err := ui.CheckRestriction(ossettings.RoamingToggle, restriction.Disabled)(ctx); err != nil {
		s.Fatal("Roaming toggle is not disabled: ", err)
	}
}
