// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularPolicyConnectionAllowUnmanaged,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can be connected and disconnected and restrict managed only cellular network works properly",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolled",
		Timeout:      13 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func CellularPolicyConnectionAllowUnmanaged(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cleanupCtx := ctx

	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("ESimPolicy"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer func() {
		policyutil.ResetChrome(cleanupCtx, fdms, cr)
		cr.Close(cleanupCtx)
	}()

	networkConfigurations, cleanupFunc, err := cellular.GetManagedProfileIccidBeforeTest(ctx)
	if err != nil {
		s.Fatal("Failed to connect to each cellular network before applying policy: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API in clean up: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyCellularNetworks: false,
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

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	managedNetworkWithBuildingIcon := nodewith.NameRegex(regexp.MustCompile(".*ManagedProfile,.*Managed by your Administrator.*")).Role(role.GenericContainer).First()
	managedDetail := nodewith.NameContaining(ossettings.ManagedEsimProfileName).ClassName("subpage-arrow").Role(role.Button)

	if err := uiauto.Combine("click to connect to the managed network and verify connected",
		ui.LeftClick(managedNetworkWithBuildingIcon),
		ui.WithTimeout(90*time.Second).LeftClick(managedDetail),
		ui.WaitUntilExists(ossettings.ConnectedStatus),
	)(ctx); err != nil {
		s.Fatal("Failed to click to connect to the managed network and verify connected: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.RoamingToggle)(ctx); err != nil {
		s.Log("Got back to the network subpage from the detail page")
		if err := ui.LeftClick(managedDetail)(ctx); err != nil {
			s.Fatal("Couldn't go to managed network detail page")
		}
	}

	if err := uiauto.Combine("In the managed network detail page, disconnect and go back",
		ui.EnsureGoneFor(ossettings.ConnectingStatus, 5*time.Second),
		ui.LeftClick(ossettings.DisconnectButton),
		ui.WaitUntilExists(ossettings.DisconnectedStatus),
		ui.LeftClick(ossettings.BackArrowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to disconnect and go back in the managed network detail page: ", err)
	}

	unmanagedNetworkDetail := nodewith.NameContaining(ossettings.UnmanagedEsimProfileName).ClassName("subpage-arrow").Role(role.Button)

	if err := uiauto.Combine("go to the unmanaged detail page, connect, and verify connected",
		ui.LeftClick(unmanagedNetworkDetail),
		ui.LeftClick(ossettings.ConnectButton),
		ui.WithTimeout(150*time.Second).WaitUntilExists(ossettings.ConnectedStatus),
	)(ctx); err != nil {
		s.Fatal("Failed to go to the unmanaged detail page, connect, and verify connected: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.RoamingToggle)(ctx); err != nil {
		s.Log("Got back to the network subpage from the detail page")
		if err := ui.LeftClick(unmanagedNetworkDetail)(ctx); err != nil {
			s.Fatal("Couldn't go to unmanaged network detail page")
		}
	}

	if err := uiauto.Combine("In the unmanaged network detail page, disconnect and go back",
		ui.EnsureGoneFor(ossettings.ConnectingStatus, 5*time.Second),
		ui.LeftClick(ossettings.DisconnectButton),
		ui.WaitUntilExists(ossettings.DisconnectedStatus),
	)(ctx); err != nil {
		s.Fatal("Failed to disconnect and go back in the unmanaged network detail page: ", err)
	}

	if err := app.Close(ctx); err != nil {
		s.Fatal("Failed to close settings app: ", err)
	}

	// Verify that the restrict managed network also works properly from quick settings
	s.Log("Start testing cellular connection from quick settings")
	if err := quicksettings.Expand(ctx, tconn); err != nil {
		s.Fatal("Fail to open quick settings")
	}

	networkFeaturePodLabelButton := nodewith.ClassName("FeaturePodLabelButton").NameContaining("network list")
	connectManagedNetwork := nodewith.NameStartingWith("Connect to " + ossettings.ManagedEsimProfileName)
	connectUnmanagedNetwork := nodewith.NameStartingWith("Connect to " + ossettings.UnmanagedEsimProfileName)
	connectingToManagedNetwork := nodewith.NameStartingWith("Connecting to " + ossettings.ManagedEsimProfileName)
	connectingToUnmanagedNetwork := nodewith.NameStartingWith("Connecting to " + ossettings.UnmanagedEsimProfileName)
	openUnmanagedNetwork := nodewith.NameStartingWith("Open settings for " + ossettings.UnmanagedEsimProfileName)

	if err := uiauto.Combine("Verify both managed and unmanaged network is connectable from quick settings",
		ui.LeftClick(networkFeaturePodLabelButton),
		ui.LeftClick(connectManagedNetwork),
		ui.WithTimeout(150*time.Second).WaitUntilGone(connectingToManagedNetwork),
		ui.LeftClick(connectUnmanagedNetwork),
		ui.WithTimeout(150*time.Second).WaitUntilGone(connectingToUnmanagedNetwork),
		ui.WaitUntilExists(openUnmanagedNetwork),
	)(ctx); err != nil {
		s.Fatal("Failed to verify both managed and unmanaged network is connectable from quick settings")
	}
}
