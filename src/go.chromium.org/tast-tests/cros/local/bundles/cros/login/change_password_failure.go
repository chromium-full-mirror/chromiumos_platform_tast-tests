// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangePasswordFailure,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks cryptohome password change flow when user does not remember the old password",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:hw_agnostic"},
		VarDeps: []string{
			"ui.gaiaPoolDefault",
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: chrome.GAIALoginTimeout + 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Credentials sync - password change failure.
			Value: "screenplay-e48269d3-5309-4db0-aafa-ffdce9a79dbf",
		}},
	})
}

func ChangePasswordFailure(ctx context.Context, s *testing.State) {
	var fakeCreds chrome.Creds
	var gaiaCreds chrome.Creds

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupCtx)

	// Isolate the step to leverage `defer` pattern.
	func() {
		var err error
		gaiaCreds, err = credconfig.PickRandomCreds(s.RequiredVar("ui.gaiaPoolDefault"))
		if err != nil {
			s.Fatal("Failed to parse creds: ", err)
		}

		fakeCreds = gaiaCreds
		// Add something to the password so when user logs in again - password change would be detected.
		fakeCreds.Pass = "fake" + fakeCreds.Pass
		cr, err := chrome.New(
			ctx, chrome.FakeLogin(fakeCreds),
			// We don't have an explicit way to control recovery feature state via
			// options, so rely on feature flag instead.
			chrome.DisableFeatures("CryptohomeRecoveryByDefaultForConsumers"))
		if err != nil {
			s.Fatal("Failed to create a user: ", err)
		}
		defer cr.Close(cleanupCtx)
		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}
	}()

	// Isolate the step to leverage `defer` pattern.
	func() {
		cr, err := chrome.New(
			ctx,
			chrome.GAIALogin(gaiaCreds),
			chrome.DontWaitForCryptohome(),
			chrome.KeepState(),
			chrome.RemoveNotification(false), // By default it waits for the user session.
			chrome.DontSkipOOBEAfterLogin())
		if err != nil {
			s.Fatal("Chrome login failed: ", err)
		}
		defer cr.Close(cleanupCtx)
		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to wait for OOBE connection: ", err)
		}
		defer oobeConn.Close()

		if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "!document.querySelector('#enter-old-password').hidden", 20*time.Second); err != nil {
			s.Fatal("Failed to wait for the gaia password changed screen: ", err)
		}
		if err := oobeConn.Eval(ctx, fmt.Sprintf("document.querySelector('#enter-old-password').$.oldPasswordInput.value = '%s'", fakeCreds.Pass), nil); err != nil {
			s.Fatal("Failed to enter old password: ", err)
		}
		if err := oobeConn.Eval(ctx, "document.querySelector('#enter-old-password').$.forgotPasswordButton.click()", nil); err != nil {
			s.Fatal("Failed to click on forgot password link: ", err)
		}

		if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "!document.querySelector('#local-data-loss-warning').hidden", 20*time.Second); err != nil {
			s.Fatal("Failed to wait for the local data loss warning: ", err)
		}

		if err := oobeConn.Eval(ctx, "document.querySelector('#local-data-loss-warning').$.proceedRemove.click()", nil); err != nil {
			s.Fatal("Failed to click reset button: ", err)
		}
		if err := oobeConn.Eval(ctx, "OobeAPI.skipPostLoginScreens()", nil); err != nil {
			// This is not fatal because sometimes it fails because Oobe shutdowns too fast after the call - which produces error.
			s.Log("Failed to call skip post login screens: ", err)
		}
		if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
			s.Fatal("Failed to wait for OOBE to be dismissed: ", err)
		}
	}()

	// Verify we can not login with the old password.
	loginWithCreds(ctx, cleanupCtx, s, fakeCreds, false)

	// Verify we can login with the new password.
	loginWithCreds(ctx, cleanupCtx, s, gaiaCreds, true)
}

func loginWithCreds(ctx, cleanupCtx context.Context, s *testing.State, creds chrome.Creds, successExpected bool) {
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting signing test API connection failed: ", err)
	}

	if err = lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 10*time.Second); err != nil {
		s.Fatal("Fail to wait for password: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)
	if err = lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass, keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if successExpected {
		if err = lockscreen.WaitForLoggedIn(ctx, tconn, chrome.LoginTimeout); err != nil {
			s.Fatal("Failed to login: ", err)
		}
	} else {
		if err := lockscreen.WaitForAuthError(ctx, tconn, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for auth error: ", err)
		}
	}
}
