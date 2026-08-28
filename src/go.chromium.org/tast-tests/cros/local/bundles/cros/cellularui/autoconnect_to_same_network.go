// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type autoconnectToSameNetworkTestParams struct {
	shouldEnableAutoconnect bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           AutoconnectToSameNetwork,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Checks that disabling and re-enabling mobile data will only reconnect if auto-connect was enabled",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic"},
		Fixture: "cellularEnforceConnectionLocal",
		Params: []testing.Param{{
			Name: "auto_connect_enabled",
			Val:  autoconnectToSameNetworkTestParams{shouldEnableAutoconnect: true},
		}, {
			Name: "auto_connect_disabled",
			Val:  autoconnectToSameNetworkTestParams{shouldEnableAutoconnect: false},
		}},
		Timeout: 2*time.Minute + cellular.ConnectTimeout,
	})
}

func AutoconnectToSameNetwork(ctx context.Context, s *testing.State) {
	shouldEnableAutoconnect := s.Param().(autoconnectToSameNetworkTestParams).shouldEnableAutoconnect

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "mobile_data_subpage_ui_dump")

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, iccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}

	autoconnectChanged, err := helper.SetServiceAutoConnect(ctx, shouldEnableAutoconnect)
	if err != nil {
		s.Fatalf("Failed to set auto-connect to %t", shouldEnableAutoconnect)
	}
	resetAutoconnect := func() {
		if autoconnectChanged {
			if _, err := helper.SetServiceAutoConnect(cleanupCtx, !shouldEnableAutoconnect); err != nil {
				s.Fatalf("Failed to set auto-connect back to %t", !shouldEnableAutoconnect)
			}
		}
	}
	defer resetAutoconnect()

	if err := ossettings.VerifyAutoconnectStateOfActiveNetwork(ctx, tconn, shouldEnableAutoconnect); err != nil {
		s.Fatal("Failed to verify auto-connect toggle of network: ", err)
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
		ui.WaitUntilEnabled(ossettings.MobileDataToggle),
		mdp.LeftClick(ossettings.MobileDataToggle),
		ui.WaitUntilCheckedState(ossettings.MobileDataToggle, true),
	)(ctx); err != nil {
		s.Fatal("Failed to turn off and turn on mobile data: ", err)
	}

	// Ensure mobile data page is open as it may have navigated away.
	_, err = ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for profile refresh: ", err)
	}

	secondIccid, _ := helper.GetCurrentICCID(ctx)
	sameNetworkIsConnected := iccid == secondIccid && helper.IsConnected(ctx) == nil
	if sameNetworkIsConnected != shouldEnableAutoconnect {
		if shouldEnableAutoconnect {
			s.Fatal("Cellular network should have been auto-connected but is disconnected")
		} else {
			s.Fatal("Cellular network should have been disconnected but is auto-connected")
		}
	}

	// Only check that the UI shows the network as connected if we expect it to be connected.
	if !shouldEnableAutoconnect {
		return
	}

	if err := ossettings.VerifyNetworkIsActive(ctx, tconn, iccid); err != nil {
		s.Fatal("Failed to verify network is active: ", err)
	}
}
