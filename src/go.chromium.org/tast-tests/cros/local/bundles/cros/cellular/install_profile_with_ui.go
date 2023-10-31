// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/cellular/esim/mojo"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InstallProfileWithUI,
		Desc:         "Installs an eSIM profile using the UI",
		Contacts:     []string{"cros-connectivity@google.com", "chadduffin@google.com"},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_test_esim"},
		Fixture:      "chromeLoggedInWithMojoTestEuiccAndSmdsSupport",
		Timeout:      12 * time.Minute,
	})
}

// InstallProfileWithUI ensures that an eSIM profile can be installed using the UI.
func InstallProfileWithUI(ctx context.Context, s *testing.State) {
	// The maximum amount of time we will wait for various eSIM installation operations.
	const scanDuration = 2 * time.Minute
	const installDuration = 5 * time.Minute
	const uninstallDuration = 2 * time.Minute

	fixtData := s.FixtValue().(*mojo.FixtData)
	cr := fixtData.Cr
	euicc := fixtData.Euicc

	crosNetworkConfig, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer crosNetworkConfig.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	_, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, euicc.Eid, 1)
	if cleanupFunc != nil {
		defer cleanupFunc(cleanupCtx)
	}
	if err != nil {
		s.Fatal("Failed to fetch the Stork profile: ", err)
	}

	if err := crosNetworkConfig.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to wait for the cellular device to become uninhibited before resetting EUICC: ", err)
	}
	if err := resetEuiccMemory(ctx); err != nil {
		s.Fatal("Failed to reset EUICC memory: ", err)
	}
	if err := crosNetworkConfig.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to wait for the cellular device to become uninhibited after resetting EUICC: ", err)
	}

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to navigate to network detailed view: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(quicksettings.AddCellularButton)(ctx); err != nil {
		s.Fatal("Failed to select the \"Add eSIM\" button: ", err)
	}

	var dialog = nodewith.NameContaining("Mobile data").Role(role.RootWebArea)
	var dialogTitle = nodewith.NameContaining("Automatically scan for available eSIM profiles?").Role(role.StaticText).Ancestor(dialog)
	if err := ui.WaitUntilExists(dialogTitle)(ctx); err != nil {
		s.Fatal("Failed to wait for the \"Add eSIM\" dialog to be visible: ", err)
	}

	var scanButton = nodewith.NameContaining("Scan").Role(role.Button).Ancestor(dialog)
	var testProfileOption = nodewith.NameContaining("Test Profile").Role(role.StaticText).First().Ancestor(dialog)
	var installButton = nodewith.NameContaining("Next").Role(role.Button).Ancestor(dialog)
	var successPage = nodewith.NameContaining("Network added").Role(role.StaticText).Ancestor(dialog)
	var doneButton = nodewith.NameContaining("Done").Role(role.Button).Ancestor(dialog)

	// Defer a call to reset the test EUICC at the end of the test in case we fail to remove the profile using the UI.
	defer func(ctx context.Context) {
		if err := resetEuiccMemory(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to reset EUICC")
		}
	}(cleanupCtx)

	if err := uiauto.Combine("Discover and install profile",
		ui.LeftClick(scanButton),
		ui.WithTimeout(scanDuration).LeftClick(testProfileOption),
		ui.LeftClick(installButton),
		ui.WithTimeout(installDuration).WaitUntilExists(successPage),
		ui.LeftClick(doneButton),
	)(ctx); err != nil {
		s.Fatal("Failed to discover available profiles: ", err)
	}

	// After installing an eSIM profile the cellular device will become inhibited while we attempt to connect to the profile.
	// Wait for the cellular device to no longer be inhibited before continuing.
	if err := crosNetworkConfig.WaitForCellularDeviceUninhibited(ctx); err != nil {
		s.Fatal("Failed to wait for the cellular device to become uninhibited: ", err)
	}

	if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, stork.ServiceProviderNameValue, netconfigtypes.Cellular); err != nil {
		s.Fatal("Failed to navigate to the network detailed page of the installed profile: ", err)
	}

	var testProfile = nodewith.NameContaining(stork.ServiceProviderNameValue).Role(role.GenericContainer).First()

	if err := uiauto.Combine("Remove installed profile",
		ui.LeftClick(ossettings.MoreActionsBtn),
		ui.LeftClick(ossettings.RemoveProfileOption),
		ui.LeftClick(ossettings.RemoveProfileButton),
		ui.WithTimeout(uninstallDuration).WaitUntilGone(testProfile),
	)(ctx); err != nil {
		s.Fatal("Failed to discover available profiles: ", err)
	}
}

func resetEuiccMemory(ctx context.Context) error {
	euicc, _, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		return err
	}
	return euicc.ResetMemory(ctx)
}
