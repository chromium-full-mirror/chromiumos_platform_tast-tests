// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExpectedElementsOnESimDetailPage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that expected elements are shown on the detail view page of an eSIM profile",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      30 * time.Second,
	})
}

func ExpectedElementsOnESimDetailPage(ctx context.Context, s *testing.State) {
	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get iccid: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(ctx)

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	ui := uiauto.New(tconn)

	networkNameLabel := nodewith.Name(networkName).Role(role.StaticText).First()

	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(networkNameLabel)(ctx); err != nil {
		s.Fatal("Failed to find network name label: ", err)
	}

	if err := ui.Exists(ossettings.AutoconnectToggle)(ctx); err != nil {
		s.Fatal("Failed to find autoconnect toggle: ", err)
	}

	if err := ui.Exists(ossettings.RoamingToggle)(ctx); err != nil {
		s.Fatal("Failed to find roaming toggle: ", err)
	}

	if err := ui.Exists(ossettings.CellularAdvanced)(ctx); err != nil {
		s.Fatal("Failed to find advanced button: ", err)
	}

	if err := ui.Exists(ossettings.CellularNetwork)(ctx); err != nil {
		s.Fatal("Failed to find network button: ", err)
	}

	if err := ui.Exists(ossettings.CellularProxy)(ctx); err != nil {
		s.Fatal("Failed to find proxy button: ", err)
	}
}
