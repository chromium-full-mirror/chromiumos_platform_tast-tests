// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           SimLockPolicyLockSettingOn,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Test the notification flow that's triggered when the 'Lock SIM' setting is turned on before the policy is turned on",
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

func SimLockPolicyLockSettingOn(ctx context.Context, s *testing.State) {
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
	if currentPin == "" {
		// Do graceful exit, not to run tests on unknown pin duts.
		s.Fatal("Unable to find PIN code for ICCID: ", iccid)
	}
	if currentPuk == "" {
		// Do graceful exit, not to run tests on unknown puk duts.
		s.Fatal("Unable to find PUK code for ICCID: ", iccid)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		// Unlock and disable pin lock.
		if err = helper.ClearSIMLock(ctx, currentPin, currentPuk); err != nil {
			s.Fatal("Failed to clear PIN/PUK lock: ", err)
		}
	}(cleanupCtx)

	// Check if pin enabled and locked/set.
	if helper.IsSimLockEnabled(ctx) || helper.IsSimPinLocked(ctx) {
		// Disable pin.
		if err = helper.Device.RequirePin(ctx, currentPin, false); err != nil {
			s.Fatal("Failed to disable lock: ", err)
		}
	}

	s.Log("Attempting to enable SIM lock with correct pin")
	if err = helper.Device.RequirePin(ctx, currentPin, true); err != nil {
		s.Fatal("Failed to enable pin, mostly default pin needs to set on dut: ", err)
	}

	defer func(ctx context.Context) {
		// Check if pin enabled and locked/set.
		if helper.IsSimLockEnabled(ctx) || helper.IsSimPinLocked(ctx) {
			// Disable pin.
			if err = helper.Device.RequirePin(ctx, currentPin, false); err != nil {
				s.Fatal("Failed to turn off lock: ", err)
			}
			s.Fatal("PIN lock was not disabled through the UI")
		}
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)

	globalConfig := &policy.ONCGlobalNetworkConfiguration{
		AllowCellularSimLock: false,
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

	const notificationTitle = "Turn off \"Lock SIM\" setting"
	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, ash.WaitTitle(notificationTitle)); err != nil {
		s.Fatalf("Failed waiting for %v: %v", notificationTitle, err)
	}

	ui := uiauto.New(tconn)
	popup := nodewith.Role(role.Window).HasClass("ash/message_center/MessagePopup")
	if err := ui.LeftClick(popup)(ctx); err != nil {
		s.Fatal("Failed to click on notification: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatal("Failed to show settings: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.EnterButton)(ctx); err != nil {
		s.Fatal("Waiting for EnterButton: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.VisibilityButton)(ctx); err != nil {
		s.Fatal("Waiting for VisibilityButton: ", err)
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	if err := keyboard.Type(ctx, currentPin); err != nil {
		s.Fatal("Could not type PIN: ", err)
	}

	if err := ui.LeftClick(ossettings.EnterButton)(ctx); err != nil {
		s.Fatal("Failed to click Enter button: ", err)
	}

	// Await for Visibility button to close.
	if err := ui.WaitUntilGone(ossettings.EnterButton)(ctx); err != nil {
		s.Fatal("Failed wait until Enter button is gone: ", err)
	}
}
