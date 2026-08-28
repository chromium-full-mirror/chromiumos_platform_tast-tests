// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectToRoamingSim,
		Desc: "Disable roaming on a roaming sim and verify connecting to network fails",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		// ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		BugComponent:   "b:1578688",
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		SoftwareDeps:   []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:           []string{"group:cellular", "cellular_sim_roaming"},
		Fixture: "cellularWithFunctioningRoamingSim",
		Timeout: 3 * time.Minute,
	})
}

func ConnectToRoamingSim(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data sub page: ", err)
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "ui_dump")

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)

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

	cleanup, err := cellular.SetRoamingPolicy(ctx, false, false)
	if err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}
	defer cleanup(cleanupCtx)

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
