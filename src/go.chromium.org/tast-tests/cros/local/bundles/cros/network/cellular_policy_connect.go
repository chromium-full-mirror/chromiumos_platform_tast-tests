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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type eSimPolicyTestParam struct {
	useManagedProfile bool
	allowManagedOnly  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularPolicyConnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can be connected and disconnected and restrict managed only cellular network works properly",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_prod_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolled",
		Timeout:      8 * time.Minute,
		Params: []testing.Param{
			{
				Name: "managed_network_with_allow_only_policy",
				Val: eSimPolicyTestParam{
					useManagedProfile: true,
					allowManagedOnly:  true,
				},
			},
			{
				Name: "managed_network_without_allow_only_policy",
				Val: eSimPolicyTestParam{
					useManagedProfile: true,
					allowManagedOnly:  false,
				},
			},
			{
				Name: "unmanaged_network_without_allow_only_policy",
				Val: eSimPolicyTestParam{
					useManagedProfile: false,
					allowManagedOnly:  false,
				},
			},
			{
				Name: "unmanaged_network_with_allow_only_policy",
				Val: eSimPolicyTestParam{
					useManagedProfile: false,
					allowManagedOnly:  true,
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func CellularPolicyConnect(ctx context.Context, s *testing.State) {
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

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
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

	testOpts := s.Param().(eSimPolicyTestParam)

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
		AllowOnlyPolicyCellularNetworks: testOpts.allowManagedOnly,
	}

	var deviceNetworkPolicy *policy.DeviceOpenNetworkConfiguration
	if testOpts.useManagedProfile {
		deviceNetworkPolicy = &policy.DeviceOpenNetworkConfiguration{
			Val: &policy.ONC{
				GlobalNetworkConfiguration: globalConfig,
				NetworkConfigurations:      networkConfigurations,
			},
		}
	} else {
		deviceNetworkPolicy = &policy.DeviceOpenNetworkConfiguration{
			Val: &policy.ONC{
				GlobalNetworkConfiguration: globalConfig,
			},
		}
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

	if testOpts.useManagedProfile || !testOpts.allowManagedOnly {
		// This verifies that the building con appears properly.
		networkWithBuildingIcon := nodewith.NameRegex(regexp.MustCompile(".*Managed by your Administrator.*")).Role(role.GenericContainer).First()
		unmanagedNetwork := nodewith.NameContaining(networkName).Role(role.GenericContainer).First()
		if testOpts.useManagedProfile {
			if err := ui.LeftClick(networkWithBuildingIcon)(ctx); err != nil {
				s.Fatal("Failed to click to connect to the managed network: ", err)
			}
		} else {
			if err := ui.LeftClick(unmanagedNetwork)(ctx); err != nil {
				s.Fatal("Failed to click to connect to the unmanaged network: ", err)
			}
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

		if err := uiauto.Combine("In the network detail page, disconnect and go back",
			ui.WithTimeout(90*time.Second).LeftClick(ossettings.DisconnectButton),
			ui.WaitUntilExists(ossettings.DisconnectedStatus),
			ui.LeftClick(ossettings.BackArrowBtn),
		)(ctx); err != nil {
			s.Fatal("Failed to disconnect and go back from the managed network detail page: ", err)
		}
	} else {
		unmanagedNetwork := nodewith.NameContaining(networkName).Role(role.GenericContainer).First()

		// Click on unmanaged cellular should not attempt to make a connection, and it should bring you to the detail page
		if err := uiauto.Combine("Click on unmanaged network and verify it doesn't get connected",
			ui.LeftClick(unmanagedNetwork),
			ui.WithTimeout(5*time.Second).WaitUntilExists(ossettings.DisconnectedStatus),
			ui.EnsureGoneFor(ossettings.ConnectButton, 5*time.Second),
		)(ctx); err != nil {
			s.Fatal("Failed to click on unmanaged network and verify it doesn't get connected: ", err)
		}
	}

	// Verify that the restrict managed network also works properly from quick settings
	s.Log("Start testing cellular connection from quick settings")
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the detailed Network view: ", err)
	}

	connectNetwork := nodewith.NameStartingWith("Connect to " + networkName)
	connectingToNetwork := nodewith.NameStartingWith("Connecting to " + networkName)
	openNetwork := nodewith.NameStartingWith("Open settings for " + networkName)
	disableTooltip := nodewith.NameStartingWith("This network is disabled by your administrator").Role(role.Tooltip)

	if testOpts.useManagedProfile || !testOpts.allowManagedOnly {
		if err := uiauto.Combine("Verify network is connectable through quick settings",
			ui.LeftClick(connectNetwork),
			ui.WithTimeout(90*time.Second).WaitUntilGone(connectingToNetwork),
			ui.WaitUntilExists(openNetwork),
		)(ctx); err != nil {
			s.Fatal("Failed to verify network is connectable through quick settings: ", err)
		}
	} else {
		if err := uiauto.Combine("Verify unmanaged network is not connectable in quick settings",
			ui.LeftClick(connectNetwork),
			ui.WaitUntilExists(disableTooltip),
			ui.EnsureGoneFor(openNetwork, 3*time.Second),
		)(ctx); err != nil {
			s.Fatal("Failed to verify unmanaged network is not connectable in quick settings: ", err)
		}
	}
}
