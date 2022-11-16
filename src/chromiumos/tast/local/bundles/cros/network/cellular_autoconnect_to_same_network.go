// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularAutoconnectToSameNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that when mobile data is turned off then back on, the previously connected network autoconnects",
		Contacts: []string{
			"hsuregan@google.com",
			"cros-connectivity@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_e2e_unstable", "cellular_sim_active"},
		Fixture:      "cellular",
		Vars:         []string{"autotest_host_info_labels"},
	})
}

func CellularAutoconnectToSameNetwork(ctx context.Context, s *testing.State) {
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

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, iccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}

	wasAutoconnectChanged, err := helper.SetServiceAutoConnect(ctx, true)
	if err != nil {
		s.Fatal("Failed to set autoconnect to false")
	}
	resetAutoconnect := func() {
		if wasAutoconnectChanged {
			if _, err := helper.SetServiceAutoConnect(ctx, false); err != nil {
				s.Fatal("Failed to set autoconnect back to true")
			}
		}
	}
	defer resetAutoconnect()

	if err := ossettings.VerifyAutoconnectStateOfActiveNetwork(ctx, tconn, true); err != nil {
		s.Fatal("Failed to verify autoconnect toggle of network: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Turn off and turn on mobile data toggle",
		mdp.LeftClick(ossettings.MobileDataToggle),
		ui.WaitUntilCheckedState(ossettings.MobileDataToggle, false),
		// MobileDataToggle dialog has a heuristic to determine
		// unintended clicks, which includes ignoring events
		// that happen soon after the toggle changes. Add a
		// delay before clicking the MobileDataToggle again.
		action.Sleep(5*time.Second),
		mdp.LeftClick(ossettings.MobileDataToggle),
		ui.WaitUntilCheckedState(ossettings.MobileDataToggle, true),
	)(ctx); err != nil {
		s.Fatal("Failed to turn off and turn on mobile data: ", err)
	}

	// Ensure mobile data page is open as it may have navigated away.
	mdp, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for profile refresh: ", err)
	}

	secondIccid, err := helper.GetCurrentICCID(ctx)
	if iccid != secondIccid || helper.IsConnected(ctx) != nil {
		s.Fatal("The same network did not autoconnect")
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, iccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}
}
