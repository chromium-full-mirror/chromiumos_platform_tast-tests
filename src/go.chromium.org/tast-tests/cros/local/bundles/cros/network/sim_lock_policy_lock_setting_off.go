// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SimLockPolicyLockSettingOff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that SIM PIN locking is disabled when the SIM lock policy is applied",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_pinlock", "cellular_e2e"},
		Fixture:      "cellularWithFakeDMSEnrolledAndSIMLockCleared",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: 9 * time.Minute,
	})
}

func SimLockPolicyLockSettingOff(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("SimLockPolicy"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	// Perform clean up
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Enable and get service to set autoconnect based on test parameters.
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable modem")
	}

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	currentPin, currentPuk, err := helper.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get Pin and Puk: ", err)
	}
	if currentPuk == "" {
		// Do graceful exit, not to run tests on unknown puk duts.
		s.Fatalf("Unable to find PUK code for ICCID : %s, skipping the test", iccid)
	}

	// Check if pin enabled and locked/set.
	if helper.IsSimLockEnabled(ctx) || helper.IsSimPinLocked(ctx) {
		// Disable pin.
		if err = helper.Device.RequirePin(ctx, currentPin, false); err != nil {
			s.Fatal("Failed to disable lock: ", err)
		}
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get name: ", err)
	}

	connectedProfile := nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*Details"))
	var networkNameDetail = nodewith.NameContaining(networkName).ClassName("subpage-arrow").Role(role.Button).Ancestor(connectedProfile.First())

	if err != nil {
		s.Fatal("Failed to ensure sim unlocked: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API in clean up: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	for _, param := range []struct {
		name                 string // subtest name.
		allowCellularSimLock bool   // Whether or not admin allow SIM lock
	}{
		{
			name:                 "Allow SIM lock",
			allowCellularSimLock: true,
		},
		{
			name:                 "Prohibit SIM lock",
			allowCellularSimLock: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			globalConfig := &policy.ONCGlobalNetworkConfiguration{
				AllowCellularSimLock: param.allowCellularSimLock,
			}

			deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
				Val: &policy.ONC{
					GlobalNetworkConfiguration: globalConfig,
					NetworkConfigurations:      []*policy.ONCNetworkConfiguration{},
				},
			}

			// Apply Global Network Configuration.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
				s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
			}
			app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
			if err != nil {
				s.Fatal("Failed to open mobile data subpage: ", err)
			}
			ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

			refreshProfileText := nodewith.NameStartingWith("Refreshing profile list").Role(role.StaticText)
			if err := app.WithTimeout(5 * time.Second).WaitUntilExists(refreshProfileText)(ctx); err == nil {
				s.Log("Wait until refresh profile finishes")
				if err := app.WithTimeout(5 * time.Minute).WaitUntilGone(refreshProfileText)(ctx); err != nil {
					s.Fatal("Failed to wait until refresh profile complete: ", err)
				}
			}

			if err := ui.WithTimeout(3 * time.Minute).WaitUntilExists(networkNameDetail)(ctx); err != nil {
				s.Fatal("Could not find connected mobile network: ", err)
			}

			if err := uiauto.Combine("Go to detail page and expand advanced section",
				ui.LeftClick(networkNameDetail),
				ui.LeftClick(ossettings.CellularAdvanced),
			)(ctx); err != nil {
				s.Fatal("Failed: ", err)
			}

			if param.allowCellularSimLock {
				if err := ui.CheckRestriction(ossettings.LockSimToggle, restriction.None)(ctx); err != nil {
					s.Fatal("Lock SIM card setting is disabled: ", err)
				}
			} else {
				if err := ui.CheckRestriction(ossettings.LockSimToggle, restriction.Disabled)(ctx); err != nil {
					s.Fatal("Lock SIM card setting is not disabled: ", err)
				}
			}

			if err := app.Close(ctx); err != nil {
				s.Fatal("Failed to close settings app: ", err)
			}
		})
	}
}
