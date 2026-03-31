// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectDisconnect,
		Desc: "Tests that user can connect and disconnect from Cellular via OS Settings and Quick Settings",
		Contacts: []string{
			"cros-device-enablement@google.com",
		},
		BugComponent:   "b:1131774", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > Cellular
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "cellularEnforceConnectionLocal",
		Timeout:        2 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func ConnectDisconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get network name: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings_ui_dump")

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to active cellular network detail page view: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.DisconnectButton)(ctx); err != nil {
		s.Fatal("Failed to find Disconnect button in OS Settings: ", err)
	}

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the network section of Quick Settings: ", err)
	}

	networkDetailedView, err := quicksettings.NetworkDetailedView(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get network detailed view: ", err)
	}

	// If there are multiple cellular networks with the same name, the currently connected network will always appear first in the list.
	cellularNetworkQuickSettingsView := nodewith.Role(role.Button).NameRegex(regexp.MustCompile(networkName)).Ancestor(networkDetailedView).First()
	connectedQuickSettingsLabel := nodewith.Role(role.StaticText).NameContaining("Connected").Ancestor(cellularNetworkQuickSettingsView)

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(connectedQuickSettingsLabel)(ctx); err != nil {
		s.Fatal("Failed to verify network is connected in Quick Settings: ", err)
	}

	// Quicksettings may be blocking the settings view.
	if err := quicksettings.Hide(ctx, tconn); err != nil {
		s.Fatal("Failed to hide Quicksettings: ", err)
	}

	if err := uiauto.Combine("Disconnect network in OS Settings",
		mdp.LeftClick(ossettings.DisconnectButton.Focusable()),
		mdp.WithTimeout(15*time.Second).WaitUntilExists(ossettings.ConnectButton.Focusable()),
	)(ctx); err != nil {
		s.Fatal("Failed to disconnect cellular network in OS Settings: ", err)
	}

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the network section of Quick Settings: ", err)
	}

	if err := ui.Exists(connectedQuickSettingsLabel)(ctx); err == nil {
		s.Fatal("Failed to verify that network is no longer connected in Quick Settings: ", err)
	}

	if err := uiauto.Combine("Connect to network in Quick Settings and verify connection in OS Settings",
		ui.LeftClick(cellularNetworkQuickSettingsView),
		ui.WithTimeout(15*time.Second).WaitUntilExists(connectedQuickSettingsLabel),
		mdp.WithTimeout(15*time.Second).WaitUntilExists(ossettings.DisconnectButton),
	)(ctx); err != nil {
		s.Fatal("Failed to connect to network in Quick Settings and verify connection in OS Settings: ", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to click back button in OS Settings: ", err)
	}

	if err := mdp.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.MobileDataToggle)(ctx); err != nil {
		s.Fatal("Failed to find mobile data toggle button in OS Settings: ", err)
	}

	if err := mdp.Exists(ossettings.DisconnectButton)(ctx); err == nil {
		s.Fatal("Failed navigate away from cellular details page in OS Settings: ", err)
	}

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to the network section of Quick Settings: ", err)
	}

	if err := uiauto.Combine("Click connected cellular network in Quick Settings to navigate to network details view in OS Settings",
		ui.LeftClick(cellularNetworkQuickSettingsView),
		mdp.WithTimeout(15*time.Second).WaitUntilExists(ossettings.DisconnectButton),
	)(ctx); err != nil {
		s.Fatal("Failed to verify that clicking on a connected cellular network in Quick settings navigates to network details view in OS Settings : ", err)
	}
}
