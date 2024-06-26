// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ConnectToRoamingSim,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Disable roaming on a roaming sim and verify connecting to network fails",
		Contacts: []string{
			"chromeos-connectivity-cienet-external@google.com",
			"alfred.yu@cienet.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_roaming"},
		Fixture:      "cellularWithFunctioningRoamingSim",
		Timeout:      3 * time.Minute,
	})
}

func ConnectToRoamingSim(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
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

	if err := uiauto.Combine("Disconnect from network",
		ui.WaitUntilExists(ossettings.DisconnectButton),
		ui.LeftClick(ossettings.DisconnectButton),
		ui.WaitUntilExists(ossettings.DisconnectedStatus),
	)(ctx); err != nil {
		s.Fatal(err, "failed to disconnect from network: ", err)
	}

	if err := ui.LeftClick(ossettings.ConnectButton)(ctx); err != nil {
		s.Fatal("Failed to click the connect button: ", err)
	}

	const notificationTitle = "Network connection error"
	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle)); err == nil {
		ui.LeftClick(ossettings.ConnectButton)(ctx)

		notification, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle))
		if err == nil {
			s.Fatal("Network connection failed: ", notification.Message)
		}
	}

	_, err = ui.FindAnyExists(ctx, ossettings.ConnectedStatus, ossettings.LimitedConnectivityStatus)
	if err != nil {
		s.Fatal("Failed to connect and verify connected: ", err)
	}

	err = cellular.SetRoamingPolicy(ctx, false, false)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.DisconnectedStatus)(ctx); err != nil {
		s.Fatal("Automatic disconnection failed: ", err)
	}

	if err := ui.LeftClick(ossettings.ConnectButton)(ctx); err != nil {
		s.Fatal("Failed to connect and verify connected: ", err)
	}

	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle)); err != nil {
		s.Fatalf("Failed waiting for %v: %v", notificationTitle, err)
	}
}
