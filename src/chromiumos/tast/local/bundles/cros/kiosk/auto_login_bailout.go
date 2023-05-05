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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutoLoginBailout,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Cancel Kiosk app launch on the splash screen",
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
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration,
		Fixture:      fixture.FakeDMSEnrolled,
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

func AutoLoginBailout(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, _, err := kioskmode.New(
		ctx,
		fdms,
		signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
		kioskmode.SkipSuccessfulLaunchCheck(),
		kioskmode.ExtraChromeOptions(
			chrome.LoadSigninProfileExtension(signinTestExtensionManifestKey),
			chrome.ExtraArgs("--kiosk-splash-screen-min-time-seconds=60"),
		),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close Kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitForSplashScreenShowing(); err != nil {
		s.Error("Failed to wait for Kiosk splash screen: ", err)
	}

	cr, err := kiosk.CancelKioskLaunch(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.LoadSigninProfileExtension(signinTestExtensionManifestKey),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to cancel Kiosk launch: ", err)
	}

	if err := verifyKioskCanceledToastShown(ctx, cr); err != nil {
		s.Fatal("Failed to verify the canceled toast: ", err)
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
