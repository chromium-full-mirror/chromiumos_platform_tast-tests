// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularConnectedStatus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that active primary SIM is displayed correctly",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_dual_active"},
		Fixture:      "cellular",
	})
}

func CellularConnectedStatus(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
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

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	firstIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, firstIccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}

	if err := mdp.WaitUntilExists(ossettings.NotActiveCellularBtn)(ctx); err != nil {
		s.Fatal("Failed to find not connected network(s): ", err)
	}
	if err := mdp.LeftClick(ossettings.NotActiveCellularRows.First())(ctx); err != nil {
		s.Fatal("Failed to click into not active cellular row: ", err)
	}
	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	// Ensure mobile data page is open as it may have navigated away.
	mdp, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	secondIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if firstIccid == secondIccid {
		s.Fatal("Failed to connect to a different cellular network")
	}

	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, secondIccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}
}
