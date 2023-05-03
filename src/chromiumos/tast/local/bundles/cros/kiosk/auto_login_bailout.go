// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/kioskmode"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutoLoginBailout,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Stop a kiosk app launch on a splash screen",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"pbond@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome", "lacros"},
		Fixture:      fixture.KioskAutoLaunchCleanup,
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

func AutoLoginBailout(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	chromeOptions := chrome.ExtraArgs("--kiosk-splash-screen-min-time-seconds=60")

	kiosk, _, err := kioskmode.DeprecatedNew(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
		kioskmode.SkipSuccessfulLaunchCheck(),
		kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
			chromeOptions,
		),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}

	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(ctx)

	if err := kiosk.WaitForSplashScreenShowing(); err != nil {
		s.Error("Failed to wait for kiosk splash screen: ", err)
	}

	cr, err := kiosk.CancelKioskLaunch(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to connect to new chrome instance: ", err)
	}

	if err := verifyKioskCanceledToastShown(ctx, cr); err != nil {
		s.Fatal("Failed to verify the canceled toast")
	}
}

func verifyKioskCanceledToastShown(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to signin extension")
	}
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name("Kiosk application launch canceled."))(ctx); err != nil {
		return errors.Wrap(err, "failed to find canceled toast")
	}
	return nil
}
