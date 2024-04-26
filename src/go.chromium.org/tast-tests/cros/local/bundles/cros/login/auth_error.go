// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AuthError,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that auth error disappears when user perform some action",
		Contacts: []string{
			"cros-lurs@google.com",
			"rrsilva@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      chrome.LoginTimeout + 90*time.Second,
		Params: []testing.Param{{
			Val: false,
		}, {
			Name:      "learn_more_help_dialog",
			Val:       true,
			ExtraAttr: []string{"group:mainline", "informational"},
		}},
	})
}

func AuthError(ctx context.Context, s *testing.State) {
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Create user on the device.
	cr, err := chrome.New(ctx)
	defer userutil.ResetUsers(cleanupContext)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	creds := cr.Creds()
	cr.Close(ctx)

	cr, err = chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))

	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}
	defer cr.Close(cleanupContext)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)

	if err != nil {
		s.Fatal("Getting test Signin Profile API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupContext, s.OutDir(), s.HasError, tconn)

	if err := lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for the password field: ", err)
	}
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(cleanupContext)

	// Enter wrong password
	if err := lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass+"fake", keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := ui.WaitUntilExists(lockscreen.AuthErrorFinder)(ctx); err != nil {
		s.Fatal("Failed to wait for auth error: ", err)
	}

	// Click on user pod until auth error is gone
	if err := ui.LeftClickUntil(nodewith.ClassName("LoginAuthUserView"), ui.Gone(lockscreen.AuthErrorFinder))(ctx); err != nil {
		s.Fatal("Failed to wait for auth error gone: ", err)
	}

	// Enter wrong password again
	if err := lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass+"fake", keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if err := ui.WaitUntilExists(lockscreen.ConsecutiveAuthErrorFinder)(ctx); err != nil {
		s.Fatal("Failed to wait for consecutive auth error: ", err)
	}

	checkLearnMoreDialog := s.Param().(bool)

	if !checkLearnMoreDialog {
		// Typing password should make the auth error gone
		if err := lockscreen.TypePassword(ctx, tconn, creds.User, creds.Pass, keyboard); err != nil {
			s.Fatal("Failed to type password: ", err)
		}

		if err := ui.WaitUntilGone(lockscreen.ConsecutiveAuthErrorFinder)(ctx); err != nil {
			s.Fatal("Failed to wait for auth error gone: ", err)
		}
	} else {
		helpWindow := nodewith.Role(role.AlertDialog).NameStartingWith("Sign-in help")

		if err := ui.LeftClickUntil(nodewith.Role(role.Button).NameStartingWith("Learn more"),
			ui.Exists(helpWindow))(ctx); err != nil {
			s.Fatal("Failed to click Learn more button: ", err)
		}

		if err := ui.LeftClickUntil(nodewith.Role(role.Button).NameStartingWith("Close"),
			ui.Gone(helpWindow))(ctx); err != nil {
			s.Fatal("Failed to close help dialog: ", err)
		}
	}
}
