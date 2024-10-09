// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/common/pci"
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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PolicyInstall,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that managed eSIM profile can correctly be installed from device policy and the profile can not be removed or renamed",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		SoftwareDeps:   []string{"chrome"},
		Attr:           []string{"group:cellular", "cellular_sim_test_esim", "cellular_e2e"},
		Fixture:        "cellularWithFakeDMSEnrolledAndTestSIM",
		Timeout:        9 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
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

func PolicyInstall(ctx context.Context, s *testing.State) {
	s.Log("Wait for Chrome auto refresh profile list done")
	// GoBigSleepLint: Wait for Chrome auto refresh profile list done.
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

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	mdp, err := ossettings.LaunchAtMobileData(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings")

	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	activationCode, cleanupFunc, err := stork.FetchStorkProfile(ctx)
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}
	defer cleanupFunc(cleanupCtx)
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
	defer euicc.DBusObject.Call(cleanupCtx, hermesconst.EuiccMethodResetMemory, 1)
	// Capture the status of the OS settings before removing the existing profiles on the test eUICC.
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings_before_reset_memory")

	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	if err := verifyTestESimProfileNotModifiable(ctx, tconn); err != nil {
		s.Fatal("Failed to verify newly installed stork profile: ", err)
	}
	s.Log("Cellular policy test completed")
}

func verifyTestESimProfileNotModifiable(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	managedTestProfile := nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*Managed by your Administrator.*")).First()
	// testProfileDetailButton is the finder for the "Test Profile" detail subpage arrow button in the mobile data page UI.
	testProfileDetailButton := nodewith.ClassName("subpage-arrow").Role(role.Button).Ancestor(managedTestProfile)
	return uiauto.Combine("navigate to the test eSIM profile detail subpage and verify that it is not modifiable",
		ui.WithTimeout(time.Minute).WaitUntilExists(testProfileDetailButton),
		ui.WithTimeout(3*time.Minute).DoDefault(testProfileDetailButton),
		ui.WithTimeout(3*time.Second).DoDefault(tridots),
		ui.EnsureGoneFor(removeMenu, 3*time.Second),
		ui.EnsureGoneFor(renameMenu, 3*time.Second),
	)(ctx)
}
