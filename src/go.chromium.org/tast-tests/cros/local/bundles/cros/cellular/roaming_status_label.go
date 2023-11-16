// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"

	"go.chromium.org/tast/core/testing"
)

type testParameters struct {
	roamingSubLabel string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           RoamingStatusLabel,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks the roaming label status on a roaming and non roaming SIM",
		Contacts: []string{
			"cros-connectivity@google.com",
			"nikhilcn@chromium.org",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable"},
		Params: []testing.Param{
			{
				Name:      "on_roaming_sim",
				ExtraAttr: []string{"cellular_sim_roaming"},
				Val: testParameters{
					roamingSubLabel: "Currently roaming",
				},
				Fixture: "cellularWithFunctioningRoamingSim",
			},
			{
				Name:      "on_non_roaming_sim",
				ExtraAttr: []string{"cellular_sim_prod_esim"},
				Val: testParameters{
					roamingSubLabel: "Not currently roaming",
				},
				Fixture: "cellularWithFunctioningSim",
			},
		},
		Timeout: 3 * time.Minute,
	})
}

func RoamingStatusLabel(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	err = cellular.SetRoamingPolicy(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
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

	roamingSubLabel, err := app.RoamingSubLabel(ctx, cr)
	if err != nil {
		s.Fatal("Failed to fetch sublabel: ", err)
	}

	if roamingSubLabel != s.Param().(testParameters).roamingSubLabel {
		s.Fatalf("Roaming sub-label is incorrect: got %q, want %q", roamingSubLabel, s.Param().(testParameters).roamingSubLabel)
	}
}
