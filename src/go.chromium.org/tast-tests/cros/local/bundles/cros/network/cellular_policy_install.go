// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularPolicyInstall,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can correctly be installed from device policy and the profile can not be removed or renamed",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_test_esim", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolledAndTestSIM",
		Timeout:      9 * time.Minute,
	})
}

// tridots is the finder for the "More actions" button UI in cellular detail page.
var tridots = nodewith.Name("More actions").Role(role.Button)

// removeMenu is the finder for the Remove Profile menu item UI when click on the More actions menu.
var removeMenu = nodewith.Name("Remove Profile").Role(role.MenuItem)

// removeButton is the finder for the Remove eSIM Profile button UI when click on the "Remove Profile".
var removeButton = nodewith.NameStartingWith("Remove eSIM profile").Role(role.Button)

// renameButton is the finder for the Rename Profile menu item UI when click on the More action menu.
var renameMenu = nodewith.Name("Rename Profile").Role(role.MenuItem)

func CellularPolicyInstall(ctx context.Context, s *testing.State) {
	s.Log("Wait for Chrome auto refresh profile list done")
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to wait for 30 seconds: ", err)
	}
	euicc, slot, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Failed to get test euicc: ", err)
	}

	// Remove any existing profiles on test euicc
	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodResetMemory, 1).Err; err != nil {
		s.Fatal("Failed to reset test euicc: ", err)
	}
	s.Log("Reset test euicc completed")

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use test cert on eUICC: ", err)
	}
	s.Log("Set to use test cert on euicc completed")

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Start a Chrome instance that will fetch policies from the FakeDMS.
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("UseStorkSmdsServerAddress"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	}
	if slot == 1 {
		s.Log("Append CellularUseSecondEuicc feature flag")
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CellularUseSecondEuicc"))
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	refreshProfileText := nodewith.NameStartingWith("Refreshing profile list").Role(role.StaticText)
	if err := mdp.WithTimeout(5 * time.Second).WaitUntilExists(refreshProfileText)(ctx); err == nil {
		s.Log("Wait until refresh profile finishes")
		if err := mdp.WithTimeout(time.Minute).WaitUntilGone(refreshProfileText)(ctx); err != nil {
			s.Fatal("Failed to wait until refresh profile complete: ", err)
		}
	}

	activationCode, cleanupFunc, err := stork.FetchStorkProfile(ctx)
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}

	defer cleanupFunc(ctx)
	s.Log("Fetched Stork profile with activation code: ", activationCode)

	cellularONC := &policy.ONCCellular{
		SMDPAddress: string(activationCode),
	}

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyCellularNetworks: false,
	}

	deviceProfileServiceGUID := "Cellular-Device-Policy"
	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			GlobalNetworkConfiguration: globalConfig,
			NetworkConfigurations: []*policy.ONCNetworkConfiguration{
				{
					GUID:     deviceProfileServiceGUID,
					Name:     "CellularDevicePolicyName",
					Type:     "Cellular",
					Cellular: cellularONC,
				},
			},
		},
	}

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use test cert on test euicc: ", err)
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
	}
	s.Log("Applied device policy with managed cellular network configuration")
	defer euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodResetMemory, 1)

	if err := verifyTestESimProfileNotModifiable(ctx, tconn); err != nil {
		s.Fatal("Failed to verify newly installed stork profile: ", err)
	}
	s.Log("Cellular policy test completed")
}

func verifyTestESimProfileNotModifiable(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(3 * time.Second)

	managedTestProfile := nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*Managed by your Administrator.*"))
	// testProfileDetailButton is the finder for the "Test Profile" detail subpage arrow button in the mobile data page UI.
	var testProfileDetailButton = nodewith.ClassName("subpage-arrow").Role(role.Button).Ancestor(managedTestProfile.First())
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(managedTestProfile)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the newly installed test profile as a managed profile")
	}

	if err := ui.WithTimeout(3 * time.Minute).LeftClick(testProfileDetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to left click Test Profile detail button")
	}

	if err := ui.WithTimeout(3 * time.Second).LeftClick(tridots)(ctx); err != nil {
		return errors.Wrap(err, "failed to left click tridots button")
	}

	if err := ui.EnsureGoneFor(removeMenu, 3*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "should not show Remove profile in tridot menu")
	}

	if err := ui.EnsureGoneFor(renameMenu, 3*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "should not show Rename profile in tridot menu")
	}
	return nil
}
