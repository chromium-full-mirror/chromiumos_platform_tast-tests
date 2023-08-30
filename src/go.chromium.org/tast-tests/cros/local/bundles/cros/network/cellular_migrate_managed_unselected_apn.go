// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularMigrateManagedUnselectedApn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the correctness of the UI for a managed network's unselected APN that is migrated to the new UI",
		Contacts: []string{
			"cros-connectivity@google.com",
			"gordonseto@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolled",
		Timeout:      8 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func CellularMigrateManagedUnselectedApn(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cleanupCtx := ctx

	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	// Set the device to autoconnect to false.
	cleanup, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, false)
	if err != nil {
		s.Fatal("Could not initialize autoconnect to false: ", err)
	}
	defer cleanup(cleanupCtx)

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get iccid: ", err)
	}
	s.Logf("Get candidate managed iccid: %s", iccid)

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get network name: ", err)
	}
	s.Logf("Get current network name: %s", networkName)

	if _, err := helper.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect: ", err)
	}

	nickName, err := cellular.GetProfileNickNameForIccid(ctx, iccid)
	if err != nil {
		s.Fatalf("Could not get profile nick name for iccid: %s, error: %v", iccid, err)
	}

	if nickName != "" {
		networkName = nickName
	}
	s.Logf("Using network name: %s", networkName)

	// Start a Chrome instance that will fetch policies from the FakeDMS with the ApnRevamp flag disabled.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.DisableFeatures("ApnRevamp"))
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer func(ctx context.Context) {
		policyutil.ResetChrome(ctx, fdms, cr)
		cr.Close(ctx)
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API in clean up: ", err)
	}

	cellularONC := &policy.ONCCellular{
		ICCID: iccid,
	}

	deviceProfileServiceGUID := "Cellular-Managed"
	networkConfigurations := []*policy.ONCNetworkConfiguration{
		{
			GUID:     deviceProfileServiceGUID,
			Name:     "CellularManaged",
			Type:     "Cellular",
			Cellular: cellularONC,
		},
	}

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyCellularNetworks: true,
	}

	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			GlobalNetworkConfiguration: globalConfig,
			NetworkConfigurations:      networkConfigurations,
		},
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
	}
	s.Log("Applied device policy with managed cellular network configuration")
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if _, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr); err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	// GoBigSleepLint: Give some time to modem and shill to stabilize.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for 10 seconds: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// This verifies that the building icon appears properly.
	networkWithBuildingIcon := nodewith.NameRegex(regexp.MustCompile(".*Managed by your Administrator.*")).Role(role.GenericContainer).First()
	if err := ui.LeftClick(networkWithBuildingIcon)(ctx); err != nil {
		s.Fatal("Failed to click to connect to the managed network: ", err)
	}

	const notificationTitle = "Network connection error"
	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle)); err == nil {
		ui.LeftClick(ossettings.ConnectButton)(ctx)

		notification, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle))
		if err == nil {
			s.Fatal("Network connection failed: ", notification.Message)
		}
	}

	if err := ui.WithTimeout(90 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
		s.Fatal("Failed to verify the network is connected: ", err)
	}

	if err := ossettings.GoToCellularNetworkDetailPageWithNickName(ctx, tconn, networkName); err != nil {
		s.Fatal("Failed to go to the cellular network detail page view: ", err)
	}

	helper = s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	defer func(ctx context.Context) {
		if err := helper.ClearCustomAPNList(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clear cellular.CustomAPNList: ", err)
		}
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	unselectedAPNToMigrate := "UNSELECTEDAPN"
	if err := ossettings.SelectPreRevampOtherAPN(ctx, tconn, "Other"); err != nil {
		s.Fatal("Failed to select custom APN: ", err)
	}

	if err := ossettings.EnterPreRevampOtherAPNDetails(ctx, tconn, unselectedAPNToMigrate, "", "", false); err != nil {
		s.Fatal("Failed to enter custom APN: ", err)
	}

	// Select a database APN.
	apnMenuItem := nodewith.Role(role.ListBoxOption).First()
	if err := uiauto.Combine("Select first menu item",
		ui.WaitUntilExists(ossettings.AccessPointDropdown.Focusable()),
		ui.LeftClick(ossettings.AccessPointDropdown),
		ui.WaitUntilExists(apnMenuItem),
		ui.LeftClick(apnMenuItem),
	)(ctx); err != nil {
		s.Fatal("Failed to select database APN: ", err)
	}

	// Reset Shill to clear LastConnectedDefaultApn.
	if errs := helper.ResetShill(ctx); errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	// Restart Chrome with the ApnRevamp flag enabled.
	cr, err = chrome.New(ctx,
		chrome.EnableFeatures("ApnRevamp"),
		chrome.RemoveNotification(false),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	defer mdp.Close(ctx)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.GoToCellularNetworkDetailPageWithNickName(ctx, tconn, networkName); err != nil {
		s.Fatal("Failed to go to the cellular network detail page view: ", err)
	}

	if err := cellular.CheckThatApnListIsEmpty(ctx, tconn, ossettings.APNSubpageButton); err != nil {
		s.Fatal("Failed to navigate to the APN subpage and verify no APN information is shown: ", err)
	}
}
