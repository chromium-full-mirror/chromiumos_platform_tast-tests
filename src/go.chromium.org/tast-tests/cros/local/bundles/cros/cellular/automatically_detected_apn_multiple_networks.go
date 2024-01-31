// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           AutomaticallyDetectedApnMultipleNetworks,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests the correctness of the UI for automatically detected APNs in case of multiple cellular networks",
		Contacts: []string{
			"cros-connectivity@google.com",
			"nikhilcn@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_dual_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellularResetShillProfileOnPostTest",
		Timeout:      9 * time.Minute,
	})
}

func AutomaticallyDetectedApnMultipleNetworks(ctx context.Context, s *testing.State) {
	// In case roaming is required for the SIM on the device.
	if err := cellular.SetRoamingPolicy(ctx, true, true); err != nil {
		s.Fatal("Failed to set roaming property: ", err)
	}

	cr, err := chrome.New(ctx, chrome.EnableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(ctx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	if err := helper.IsConnected(ctx); err == nil {
		if _, err := helper.Disconnect(ctx); err != nil {
			s.Fatal("Failed to disconnect from current network: ", err)
		}
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(120 * time.Second)

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(ctx)

	if err := mdp.WaitUntilExists(ossettings.NotActiveCellularBtn)(ctx); err != nil {
		s.Fatal("Failed to find disconnected network(s): ", err)
	}

	cellularRow := nodewith.NameRegex(regexp.MustCompile(".*Connect")).HasClass("horizontal")

	// Finder for row to first cellular network
	firstCellularRow := cellularRow.First()
	firstCellularRowBtn := nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(firstCellularRow).Focusable()

	if err := uiauto.Combine("Go to details page of first cellular network",
		mdp.WaitUntilExists(firstCellularRowBtn),
		mdp.LeftClick(firstCellularRowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to go to details page of first cellular network: ", err)
	}

	if err := connectAndVerify(ctx, ui, s, tconn); err != nil {
		s.Fatal("Failed to connect to network: ", err)
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}

	if err := mdp.VerifyApnIsVisibleInSubtext(ctx, tconn, cr, serviceLastGoodAPNInfoApnName); err != nil {
		s.Fatal("Failed to go to verify active apn subtext: ", err)
	}

	if err := verifyAPNDetails(ctx, tconn, helper, cr, mdp); err != nil {
		s.Fatal("Failed to verify the connected APN details: ", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to go back to mobile data page: ", err)
	}

	_, err = ui.FindAnyExists(ctx, ossettings.ConnectedStatus, ossettings.LimitedConnectivityStatus)
	if err != nil {
		s.Fatal("Failed to connect and verify connected: ", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to go back to mobile data page: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	if err := ossettings.GoToFirstInactiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to disconnected network details: ", err)
	}

	if err := connectAndVerify(ctx, ui, s, tconn); err != nil {
		s.Fatal("Failed to connect to network: ", err)
	}

	if err := verifyAPNDetails(ctx, tconn, helper, cr, mdp); err != nil {
		s.Fatal("Failed to verify the connected APN details: ", err)
	}

	if err := mdp.CreateCustomAPN(ctx, &ossettings.ApnConfig{
		Name:               "INVALID_APN",
		Username:           "",
		Password:           "",
		AuthenticationType: "",
		IPType:             "",
		IsAttach:           false,
		IsDefault:          true,
	}); err != nil {
		s.Fatalf("Failed to add custom APN with name: %s, err: %v", "INVALID_APN", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to go back to mobile data page: ", err)
	}

	_, err = ui.FindAnyExists(ctx, ossettings.ConnectedStatus, ossettings.LimitedConnectivityStatus, ossettings.DisconnectedStatus)
	if err != nil {
		s.Fatal("Failed to connect and verify connected: ", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to go back to mobile data page: ", err)
	}

	if err := ossettings.GoToFirstInactiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to inactive network details: ", err)
	}

	if err := connectAndVerify(ctx, ui, s, tconn); err != nil {
		s.Fatal("Failed to connect to network: ", err)
	}
}

func connectAndVerify(ctx context.Context, ui *uiauto.Context, s *testing.State, tconn *chrome.TestConn) error {
	if err := ui.LeftClick(ossettings.ConnectButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the connect button")
	}

	const notificationTitle = "Network connection error"

	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle)); err == nil {
		ui.LeftClick(ossettings.ConnectButton)(ctx)

		_, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle))
		if err == nil {
			return errors.Wrap(err, "Network connection failed")
		}
	}

	if _, err := ui.FindAnyExists(ctx, ossettings.ConnectedStatus, ossettings.LimitedConnectivityStatus); err != nil {
		return errors.Wrap(err, "failed to connect and verify connected")
	}

	return nil
}

func verifyAPNDetails(ctx context.Context, tconn *chrome.TestConn, helper *cellular.Helper, cr *chrome.Chrome, mdp *ossettings.OSSettings) error {
	if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, false); err != nil {
		if err := ossettings.GoToActiveNetworkApnSubpage(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to go to apn subpage")
		}
	}

	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	serviceLastGoodAPNInfoApnName := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoUserFriendlyApnName]
	serviceLastGoodAPNInfoApnSource := serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnSource]
	if err != nil {
		return errors.Wrap(err, "error getting Service properties")
	}

	if err := mdp.VerifyAPNSubpageConnectedApnUI(ctx, tconn, cr, serviceLastGoodAPNInfoApnName, serviceLastGoodAPNInfoApnSource); err != nil {
		return errors.Wrap(err, "failed to verify connected APN UI")
	}

	return nil
}
