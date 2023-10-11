// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"fmt"
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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SignInWithLotsOfUsers,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test user pods are all visible in the login screen and each user can log in accordingly",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name: "10_users",
				Val:  10,
				// Each user pod takes up to 45 seconds to finish creating on low-end DUT, and login / verify also needs almost the same, which is about a minute and half for each user.
				Timeout: 10 * 1.5 * time.Minute,
			},
			{
				Name: "20_users",
				Val:  20,
				// Each user pod takes up to 45 seconds to finish creating on low-end DUT, and login / verify also needs almost the same, which is about a minute and half for each user.
				Timeout: 20 * 1.5 * time.Minute,
			},
		},
	})
}

// SignInWithLotsOfUsers tests user pods are all visible in the login screen and each user can log in accordingly.
func SignInWithLotsOfUsers(ctx context.Context, s *testing.State) {
	deviceOwner := chrome.Creds{User: "test_owner@gmail.com", Pass: "test0000"}
	if err := userutil.CreateDeviceOwner(ctx, deviceOwner.User, deviceOwner.Pass); err != nil {
		s.Fatal("Failed to create device owner: ", err)
	}

	userCount := s.Param().(int)
	testCreds := []chrome.Creds{deviceOwner}
	// Minus the device owner.
	for i := 0; i < userCount-1; i++ {
		userCreds := chrome.Creds{User: fmt.Sprintf("test_user%d@gmail.com", i), Pass: "test0000"}
		s.Log("Creating new user pod: ", userCreds.User)
		if err := userutil.CreateUser(ctx, userCreds.User, userCreds.Pass, chrome.KeepState()); err != nil {
			s.Fatal("Failed to create new user: ", err)
		}
		testCreds = append(testCreds, userCreds)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, creds := range testCreds {
		func() {
			cr, err := chrome.New(
				ctx,
				chrome.NoLogin(),
				chrome.KeepState(),
				chrome.SkipForceOnlineSignInForTesting(),
				chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
			)
			if err != nil {
				s.Fatal("Failed to start Chrome: ", err)
			}
			defer cr.Close(cleanupCtx)

			tconn, err := cr.SigninProfileTestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to connect to test API: ", err)
			}
			defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

			if err := signinAndVerify(ctx, tconn, kb, creds); err != nil {
				s.Fatal("Failed to verify sign-in function: ", err)
			}
		}()
	}
}

// signinAndVerify ensures the user is visible on the sign-in screen and verify the user can sign in with password.
func signinAndVerify(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, creds chrome.Creds) error {
	ui := uiauto.New(tconn)
	loginWindow := nodewith.Name("Login Screen").Role(role.Window)
	userButton := nodewith.Name(creds.User).Role(role.Button).Ancestor(loginWindow)
	if err := uiauto.NamedCombine(fmt.Sprintf("ensure user %q is visible", creds.User),
		ui.WaitUntilExists(userButton),
		ui.MakeVisible(userButton),
	)(ctx); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Start signing in to user ", creds.User)
	field, err := lockscreen.PasswordFieldFinder(creds.User)
	if err != nil {
		return errors.Wrap(err, "failed to get password field")
	}
	if err := ui.WithInterval(time.Second).DoDefaultUntil(userButton, ui.Exists(field))(ctx); err != nil {
		return errors.Wrap(err, "failed to focus on the user")
	}
	if err := lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass, kb); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}
	if err := lockscreen.WaitForLoggedIn(ctx, tconn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for logged in")
	}

	return nil
}
