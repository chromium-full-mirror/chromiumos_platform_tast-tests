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
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/syslog"
	"chromiumos/tast/testing"
)

// startAppParam contains test parameters for StartAppFromSignInScreen test.
type startAppParam struct {
	isLacros bool
	appName  string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         StartAppFromSignInScreen,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Adds 2 Kiosk accounts, checks if both are available then starts one of them. Depending on a test parameter, either Chrome App or Web App kiosk is launched",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"kamilszarek@google.com", // Test author
			"vkovalova@google.com",   // Lacros test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Vars:         []string{"ui.signinProfileTestExtensionManifestKey"},
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
				Name: "lacros_web_app",
				Val: startAppParam{
					isLacros: true,
					appName:  kioskmode.WebKioskTitle,
				},
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "feature_id",
					// Manually launch PWA kiosk.
					Value: "screenplay-cf0d13cd-2203-406c-a2df-1f4ad502d9e7",
				}},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "lacros_chrome_app",
				Val: startAppParam{
					isLacros: true,
					appName:  kioskmode.KioskAppBtnName,
				},
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "feature_id",
					// Manually launch chrome app kiosk.
					Value: "screenplay-749437a3-b4d5-427f-abc0-4c62d397d120",
				}},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_web_app",
				Val: startAppParam{
					isLacros: false,
					appName:  kioskmode.WebKioskTitle,
				},
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "feature_id",
					// Manually launch PWA kiosk.
					Value: "screenplay-cf0d13cd-2203-406c-a2df-1f4ad502d9e7",
				}},
			},
			{
				Name: "ash_chrome_app",
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "feature_id",
					// Manually launch chrome app kiosk.
					Value: "screenplay-749437a3-b4d5-427f-abc0-4c62d397d120",
				}},
				Val: startAppParam{
					isLacros: false,
					appName:  kioskmode.KioskAppBtnName,
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS)},
	})
}

func StartAppFromSignInScreen(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(startAppParam)
	var policies []policy.Policy
	if param.isLacros {
		policies = append(policies, &policy.LacrosAvailability{Val: "lacros_only"})
	}
	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		),
		kioskmode.PublicAccountPolicies(kioskmode.WebKioskAccountID, policies),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, policies),
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

	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		s.Fatal("Failed to start log reader: ", err)
	}
	defer reader.Close()

	// It looks like UI is not stable to interact even when polling for
	// elements. When waiting for elements and then clicking on
	// kioskmode.KioskAppBtnNode the UI element froze. I was not able to find
	// out how to overcome flakiness other than using sleep before interacting
	// with UI.
	testing.Sleep(ctx, 3*time.Second)

	ui := uiauto.New(testConn)
	if err := kioskmode.StartFromSignInScreen(ctx, ui, param.appName); err != nil {
		s.Fatal("Failed to start Kiosk application from Sign-in screen: ", err)
	}

	if err := kioskmode.ConfirmKioskStarted(ctx, reader); err != nil {
		s.Fatal("There was a problem while checking chrome logs for Kiosk related entries: ", err)
	}

	if param.isLacros {
		testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
		if _, err := lacrosproc.Root(ctx, testConn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}
}
