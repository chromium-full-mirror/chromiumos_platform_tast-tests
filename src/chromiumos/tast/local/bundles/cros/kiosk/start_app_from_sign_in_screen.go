// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/syslog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StartAppFromSignInScreen,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Adds 2 Kiosk accounts, checks if both are available then starts one of them",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"kamilszarek@google.com", // Test author
			"vkovalova@google.com",   // Lacros test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Vars:         []string{"ui.signinProfileTestExtensionManifestKey"},
		// Informational attribute can only be removed when
		// https://crbug.com/1207293 is resolved.
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name: "ash",
				Val: kioskmode.TestData{
					IsLacros: false,
				},
			},
			{
				Name: "lacros",
				Val: kioskmode.TestData{
					IsLacros: true,
					Policies: []policy.Policy{
						&policy.LacrosAvailability{Val: "lacros_only"},
					},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Manually launch chrome app kiosk.
				Value: "screenplay-749437a3-b4d5-427f-abc0-4c62d397d120",
			},
			{
				Key: "feature_id",
				// Manually launch PWA kiosk.
				Value: "screenplay-cf0d13cd-2203-406c-a2df-1f4ad502d9e7",
			}},
	})
}

func StartAppFromSignInScreen(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, param.Policies),
	)
	if err != nil {
		s.Error("Failed to start Chrome on Signin screen with set Kiosk apps: ", err)
	}

	defer kiosk.Close(ctx)

	testConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, testConn)

	// It looks like UI is not stable to interact even when polling for
	// elements. When waiting for elements and then clicking on
	// kioskmode.KioskAppBtnNode the UI element froze. I was not able to find
	// out how to overcome flakiness other than using sleep before interacting
	// with UI.
	testing.Sleep(ctx, 3*time.Second)

	localAccountsBtn := nodewith.Name("Apps").ClassName("MenuButton")
	ui := uiauto.New(testConn)
	if err := uiauto.Combine("open Kiosk application menu",
		ui.WaitUntilExists(localAccountsBtn),
		ui.LeftClick(localAccountsBtn),
		ui.WaitUntilExists(kioskmode.KioskAppBtnNode),
	)(ctx); err != nil {
		s.Fatal("Failed to open menu with local accounts: ", err)
	}

	// Get applications that show up after clicking Apps button.
	menuItems, err := ui.NodesInfo(ctx, nodewith.ClassName("MenuItemView"))
	if err != nil {
		s.Fatal("Failed to get local accounts: ", err)
	}

	const expectedLocalAccountsCount = 2
	if len(menuItems) != expectedLocalAccountsCount {
		s.Fatalf("Expected %d local accounts, but found %v app(s) %+v", expectedLocalAccountsCount, len(menuItems), menuItems)
	}

	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		s.Fatal("Failed to start log reader: ", err)
	}
	defer reader.Close()

	// When I had here only clicking the menu item that should be visible, the
	// test failed at interacting the menu item.
	if err := uiauto.Combine("close and open Kiosk application menu then click on one menu item",
		ui.WaitUntilExists(kioskmode.KioskAppBtnNode), // Wait again for the menu item to be visible.
		ui.LeftClick(kioskmode.KioskAppBtnNode),       // Launch the Kiosk app.
	)(ctx); err != nil {
		s.Fatal("Failed to start Kiosk application from Sign-in screen: ", err)
	}

	if err := kioskmode.ConfirmKioskStarted(ctx, reader); err != nil {
		s.Fatal("There was a problem while checking chrome logs for Kiosk related entries: ", err)
	}

	if param.IsLacros {
		testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
		if _, err := lacrosproc.Root(ctx, testConn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}
}
