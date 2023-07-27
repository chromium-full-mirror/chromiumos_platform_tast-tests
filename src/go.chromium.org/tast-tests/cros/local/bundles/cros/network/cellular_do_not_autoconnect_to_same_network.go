// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularDoNotAutoconnectToSameNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that when autoconnect is disabled, when mobile data is turned off then back on, the previously connected network does not autoconnect",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_e2e"},
		Fixture:      "cellular",
	})
}

func CellularDoNotAutoconnectToSameNetwork(ctx context.Context, s *testing.State) {
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

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	wasAutoconnectChanged, err := helper.SetServiceAutoConnect(ctx, false)
	if err != nil {
		s.Fatal("Failed to set autoconnect to false")
	}
	resetAutoconnect := func() {
		if wasAutoconnectChanged {
			if _, err := helper.SetServiceAutoConnect(ctx, true); err != nil {
				s.Fatal("Failed to set autoconnect back to true")
			}
		}
	}
	defer resetAutoconnect()

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, iccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
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

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for profile refresh: ", err)
	}

	secondIccid, err := helper.GetCurrentICCID(ctx)
	if iccid == secondIccid && helper.IsConnected(ctx) == nil {
		s.Fatal("Network auto-connected when it was not supposed to")
	}
}
