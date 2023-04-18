// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

// smokeTestParam contains the options that configure Smoke tests.
type smokeTestParam struct {
	// Whether this test uses lacros or ash.
	isLacros bool
	// Whether kiosk should auto launch or be manually launched.
	autoLaunch bool
	// Whether this test uses a web app or chrome app.
	isWebApp bool
}

var (
	launchChromeAppKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Launch chrome app Kiosk.
		Value: "screenplay-5e6b8c54-2eab-4ac0-a484-b9738466bb9b",
	}
	launchWebKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Launch web Kiosk.
		Value: "screenplay-d93db63f-372e-4c3b-a1ca-3f23587dbac4",
	}
	manualLaunchChromeAppKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Manually launch chrome app Kiosk.
		Value: "screenplay-749437a3-b4d5-427f-abc0-4c62d397d120",
	}
	manualLaunchWebKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Manually launch web Kiosk.
		Value: "screenplay-cf0d13cd-2203-406c-a2df-1f4ad502d9e7",
	}
	autoLaunchKioskFeature = testing.StringPair{
		Key: "feature_id",
		// Auto launch Kiosk.
		Value: "screenplay-90708eaa-cdde-4060-8ccb-eeb305485531",
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		Desc:         "Verifies core flows of Kiosk sessions",
		LacrosStatus: testing.LacrosVariantExists,
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"edmanp@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome", "lacros"},
		Vars:         []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name:             "ash_manual_chromeapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: false, isWebApp: false},
				Fixture:          fixture.FakeDMSEnrolled,
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &manualLaunchChromeAppKioskFeature}},
			{
				Name:             "ash_manual_webapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: false, isWebApp: true},
				Fixture:          fixture.FakeDMSEnrolled,
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &manualLaunchWebKioskFeature},
			},
			{
				Name:             "ash_auto_chromeapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: true, isWebApp: false},
				Fixture:          fixture.KioskAutoLaunchCleanup,
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "ash_auto_webapp",
				Val:              smokeTestParam{isLacros: false, autoLaunch: true, isWebApp: true},
				Fixture:          fixture.KioskAutoLaunchCleanup,
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "lacros_manual_chromeapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: false, isWebApp: false},
				Fixture:          fixture.FakeDMSEnrolled,
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &manualLaunchChromeAppKioskFeature},
			},
			{
				Name:             "lacros_manual_webapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: false, isWebApp: true},
				Fixture:          fixture.FakeDMSEnrolled,
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &manualLaunchWebKioskFeature},
			},
			{
				Name:             "lacros_auto_chromeapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: true, isWebApp: false},
				Fixture:          fixture.KioskAutoLaunchCleanup,
				ExtraSearchFlags: []*testing.StringPair{&launchChromeAppKioskFeature, &autoLaunchKioskFeature},
			},
			{
				Name:             "lacros_auto_webapp",
				Val:              smokeTestParam{isLacros: true, autoLaunch: true, isWebApp: true},
				Fixture:          fixture.KioskAutoLaunchCleanup,
				ExtraSearchFlags: []*testing.StringPair{&launchWebKioskFeature, &autoLaunchKioskFeature},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func (param smokeTestParam) appAccountID() string {
	if param.isWebApp {
		return kioskmode.WebKioskAccountID
	}
	return kioskmode.KioskAppAccountID
}

// appButtonName returns the name shown in the "Apps" button in the login screen.
func (param smokeTestParam) appButtonName() string {
	if param.isWebApp {
		return kioskmode.WebKioskTitle
	}
	return kioskmode.KioskAppBtnName
}

// The heading of the Chrome app.
const chromeAppHeading = "Simple Print Sample"

// appPageHeading returns the title in the app page as found in the accessibility tree.
func (param smokeTestParam) appPageHeading() string {
	if param.isWebApp {
		return kioskmode.WebKioskHeading
	}
	return chromeAppHeading
}

// kioskModeOptions returns the option slice to configure this test parameter.
func (param smokeTestParam) kioskModeOptions(signinProfileTestExtensionManifestKey string) []kioskmode.Option {
	// DefaultLocalAccount() includes device local accounts for one chrome app and one web app.
	options := []kioskmode.Option{kioskmode.DefaultLocalAccounts()}

	if param.isLacros {
		options = append(options, kioskmode.PublicAccountPolicies(
			param.appAccountID(),
			[]policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}},
		))
	}

	if param.autoLaunch {
		options = append(options, kioskmode.AutoLaunch(param.appAccountID()))
	} else {
		options = append(options, kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(signinProfileTestExtensionManifestKey),
		))
	}

	return options
}

// launchKioskAppManually clicks and launches the Kiosk app corresponding to
// this `param` under the "Apps" button from the login screen.
func launchKioskAppManually(ctx context.Context, cr *chrome.Chrome, param smokeTestParam) error {
	testing.ContextLog(ctx, "Launching Kiosk app manually")
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to get Test API connection")
	}

	// It looks like UI is not stable to interact even when polling for
	// elements. When waiting for elements and then clicking on
	// kioskmode.KioskAppBtnNode the UI element froze. I was not able to find
	// out how to overcome flakiness other than using sleep before interacting
	// with UI.
	// GoBigSleepLint: "Apps" button in sign in screen needs some time.
	testing.Sleep(ctx, 3*time.Second)

	ui := uiauto.New(tconn)
	if err := kioskmode.StartFromSignInScreen(ctx, ui, param.appButtonName()); err != nil {
		errors.Wrap(err, "failed to start Kiosk app from Sign-in screen")
	}
	return nil
}

func waitUntilKioskAppStarted(ctx context.Context, cr *chrome.Chrome, param smokeTestParam, outDir string) error {
	testing.ContextLog(ctx, "Waiting until Kiosk app started")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	appWidget := nodewith.Name(param.appPageHeading()).Role(role.Heading)
	if err := ui.WaitUntilExists(appWidget)(ctx); err != nil {
		faillog.DumpUITree(ctx, outDir, tconn)
		return errors.Wrap(err, "failed to find Kiosk app widget node")
	}
	return nil
}

func verifyLacrosIsRunning(ctx context.Context, cr *chrome.Chrome) error {
	testing.ContextLog(ctx, "Verifying lacros is running")
	tconn, err := cr.TestAPIConn(ctx)
	if _, err = lacrosproc.Root(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to get lacros process")
	}
	return nil
}

func Smoke(ctx context.Context, s *testing.State) {
	defer func(ctx context.Context) {
		// Take a screenshot if the test ends with an error.
		if s.HasError() {
			path := filepath.Join(s.OutDir(), s.TestName()+".png")
			if err := screenshot.Capture(ctx, path); err != nil {
				s.Error("Failed to take screenshot: ", err)
			}
		}
	}(ctx)

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(smokeTestParam)

	kiosk, cr, err := kioskmode.New(
		ctx, fdms, param.kioskModeOptions(s.RequiredVar("ui.signinProfileTestExtensionManifestKey"))...,
	)
	if err != nil {
		s.Fatal("Failed to create Chrome in Kiosk mode: ", err)
	}
	defer kiosk.Close(ctx)

	if !param.autoLaunch {
		if err := launchKioskAppManually(ctx, cr, param); err != nil {
			s.Fatal("Could not manual launch Kiosk app: ", err)
		}
	}

	if err := waitUntilKioskAppStarted(ctx, cr, param, s.OutDir()); err != nil {
		s.Fatal("Kiosk app did not start: ", err)
	}

	if param.isLacros {
		if err := verifyLacrosIsRunning(ctx, cr); err != nil {
			s.Fatal("Could not verify lacros is running: ", err)
		}
	}
}
