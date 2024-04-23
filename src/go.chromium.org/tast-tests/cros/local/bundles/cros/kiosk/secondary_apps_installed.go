// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// secondaryAppsInstalledParam contains the test parameters which are different
// between auto launch and manual launch.
type secondaryAppsInstalledParam struct {
	// True if Kiosk app is launched automatically. False if launched from menu.
	autoLaunch bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SecondaryAppsInstalled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if secondary apps and extensions in a Kiosk app can be installed and launched",
		Contacts:     []string{"chromeos-kiosk-eng+TAST@google.com"},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		// TODO(b/274734625): Fix and reenable test.
		// Attr: []string{
		// 	"group:golden_tier",
		// 	"group:medium_low_tier",
		// 	"group:hardware",
		// 	"group:complementary",
		//  "group:hw_agnostic",
		// },
		SoftwareDeps: []string{"reboot", "chrome", "vpd"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + time.Minute,
		Params: []testing.Param{
			{
				Name: "auto_launch",
				Val:  secondaryAppsInstalledParam{autoLaunch: true},
			},
			{
				Name: "manual_launch",
				Val:  secondaryAppsInstalledParam{autoLaunch: false},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
				},
			},
		},
	})
}

func SecondaryAppsInstalled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(secondaryAppsInstalledParam)

	// Prepare Kiosk account configuration.
	accountID := "kiosk_account@managedchrome.com"
	accountType := policy.AccountTypeKioskApp
	appID := "bkledbfligfdnfkmccllbllealecompm"
	appName := "Dev Kiosk App for multi-apps"

	account := &policy.DeviceLocalAccounts{
		Val: []policy.DeviceLocalAccountInfo{
			{
				AccountID:   &accountID,
				AccountType: &accountType,
				KioskAppInfo: &policy.KioskAppInfo{
					AppId: &appID,
				},
			},
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	// Below variables will be initialized depending on the Kiosk launch mode.
	var kiosk *kioskmode.Kiosk = nil
	var cr *chrome.Chrome = nil
	var testConn *chrome.TestConn = nil
	var err error = nil
	if param.autoLaunch {
		kiosk, cr, err = kioskmode.New(
			ctx,
			fdms,
			signinTestExtensionManifestKey,
			kioskmode.CustomLocalAccounts(account),
			kioskmode.AutoLaunch(accountID),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
		}
		defer func(ctx context.Context) {
			if err := kiosk.Close(ctx); err != nil {
				s.Error("Failed to close kiosk: ", err)
			}
		}(cleanupCtx)

		testConn, err = cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to get Test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, testConn)

	} else {
		kiosk, cr, err = kioskmode.New(
			ctx,
			fdms,
			signinTestExtensionManifestKey,
			kioskmode.CustomLocalAccounts(account),
			kioskmode.ExtraChromeOptions(chrome.LoadSigninProfileExtension(signinTestExtensionManifestKey)),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
		}
		defer func(ctx context.Context) {
			if err := kiosk.Close(ctx); err != nil {
				s.Error("Failed to close kiosk: ", err)
			}
		}(cleanupCtx)

		testConn, err = cr.SigninProfileTestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to get Test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, testConn)

		if err := kioskmode.LaunchAppManually(ctx, s.OutDir(), testConn, appName); err != nil {
			s.Fatal("Failed to start Kiosk application from Sign-in screen: ", err)
		}
	}

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if err := checkSecondaryAppAndExtension(ctx, uiauto.New(testConn)); err != nil {
		s.Fatal("Failed to check secondary app and extension: ", err)
	}
}

// checkSecondaryAppAndExtension checks if secondary apps and extensions work.
func checkSecondaryAppAndExtension(ctx context.Context, ui *uiauto.Context) error {
	const (
		extensionCheckText    = "Hello from extension: cobcmnlihjaffmjpeajkldoldonoaelf"
		secondaryAppCheckText = "App1 activated"
	)

	testing.ContextLog(ctx, "Checking secondary extension")
	extensionTestBtn := nodewith.Name("Test secondary extension").Focusable()
	extensionTestResult := nodewith.Name(extensionCheckText).Role("staticText")
	if err := uiauto.Combine("test secondary extension",
		ui.WaitUntilExists(extensionTestBtn),
		ui.LeftClick(extensionTestBtn),
		ui.WaitUntilExists(extensionTestResult),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to test secondary extension")
	}

	testing.ContextLog(ctx, "Checking secondary app")
	appTestBtn := nodewith.Name("Test secondary app").Focusable()
	appTestResult := nodewith.Name(secondaryAppCheckText).Role("staticText")
	closeSecondaryAppBtn := nodewith.Name("Close Window").Focusable()
	if err := uiauto.Combine("test secondary app",
		ui.WaitUntilExists(appTestBtn),
		ui.LeftClick(appTestBtn),
		ui.WaitUntilExists(closeSecondaryAppBtn),
		ui.LeftClick(closeSecondaryAppBtn),
		ui.WaitUntilGone(closeSecondaryAppBtn),
		ui.WaitUntilExists(appTestResult),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to test secondary app")
	}

	return nil
}
