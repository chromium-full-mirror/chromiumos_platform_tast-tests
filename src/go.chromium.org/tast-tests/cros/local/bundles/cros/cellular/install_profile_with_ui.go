// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular/esim/mojo"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           InstallProfileWithUI,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Installs an eSIM profile using the UI",
		Contacts:       []string{"cros-connectivity@google.com", "chadduffin@google.com"},
		BugComponent:   "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim"},
		Fixture:        "chromeLoggedInWithMojoTestEuiccAndSmdsSupport",
		Timeout:        20 * time.Minute,
	})
}

// InstallProfileWithUI ensures that an eSIM profile can be installed using the UI.
func InstallProfileWithUI(ctx context.Context, s *testing.State) {
	// The maximum amount of time we will wait for various eSIM installation operations.
	const scanDuration = 5 * time.Minute
	const installDuration = 5 * time.Minute
	const uninstallDuration = 2 * time.Minute

	// Install two profiles to ensure that installed profiles are not affected by the installation process.
	const profileCount = 2

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

	_, cleanupFunc, err := stork.FetchStorkProfilesForEid(ctx, euicc.Eid, profileCount)
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

	// Defer a call to reset the test EUICC at the end of the test in case we fail to remove the profile using the UI.
	defer func(ctx context.Context) {
		if err := resetEuiccMemory(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to reset EUICC")
		}
	}(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	dialog := nodewith.NameContaining("Mobile data").Role(role.RootWebArea)
	testProfileOption := nodewith.NameContaining("Test Profile").Role(role.StaticText).First()
	ui := uiauto.New(tconn)

	for i := 0; i < profileCount; i++ {
		_, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
		if err != nil {
			s.Fatal("Failed to open mobile data page: ", err)
		}

		if err := ui.LeftClick(ossettings.AddCellularButton)(ctx); err != nil {
			s.Fatal("Failed to select the \"Add eSIM\" button: ", err)
		}

		dialogTitle := nodewith.NameContaining("Automatically scan for available eSIM profiles?").Role(role.StaticText).Ancestor(dialog)
		if err := ui.WaitUntilExists(dialogTitle)(ctx); err != nil {
			s.Fatal("Failed to wait for the \"Add eSIM\" dialog to be visible: ", err)
		}

		scanButton := nodewith.NameContaining("Scan").Role(role.Button).Ancestor(dialog)
		installButton := nodewith.NameContaining("Next").Role(role.Button).Ancestor(dialog)
		successPage := nodewith.NameContaining("Network added").Role(role.StaticText).Ancestor(dialog)
		doneButton := nodewith.NameContaining("Done").Role(role.Button).Ancestor(dialog)

		if err := uiauto.Combine("Discover and install profile",
			ui.LeftClick(scanButton),
			ui.WithTimeout(scanDuration).LeftClick(testProfileOption.Ancestor(dialog)),
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
	}

	installedProfile := nodewith.NameRegex(regexp.MustCompile(`^Network \d of 2, Test Carrier`)).First()
	if err := ui.WaitUntilExists(installedProfile)(ctx); err != nil {
		s.Fatal("Failed to wait for installed profiles to be visible: ", err)
	}

	for i := 0; i < profileCount; i++ {
		testProfile := nodewith.NameContaining(stork.ServiceProviderNameValue).Role(role.GenericContainer).First()

		if err := uiauto.Combine("Remove installed profile",
			ui.LeftClick(nodewith.ClassName("subpage-arrow").Ancestor(testProfile).First()),
			ui.LeftClick(ossettings.MoreActionsBtn),
			ui.LeftClick(ossettings.RemoveProfileOption),
			ui.LeftClick(ossettings.RemoveProfileButton),
			ui.WithTimeout(uninstallDuration).WaitUntilGone(testProfile),
		)(ctx); err != nil {
			s.Fatal("Failed to remove installed profile: ", err)
		}

		if err := crosNetworkConfig.WaitForCellularDeviceUninhibited(ctx); err != nil {
			s.Fatal("Failed to wait for the cellular device to become uninhibited: ", err)
		}
	}
}

func resetEuiccMemory(ctx context.Context) error {
	euicc, _, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		return err
	}
	return euicc.ResetMemory(ctx)
}
