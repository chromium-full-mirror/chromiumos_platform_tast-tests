// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RemoveUserOnSigninScreen,
		Desc: "Checks if users can be removed on the sign in screen",
		Contacts: []string{
			"cros-lurs@google.com",
			"antrim@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		// This test
		// * creates three users,
		// * launches chrome twice,
		// * does some fast ui operations.
		// We also need some time for cleanup.
		Timeout:      5*chrome.LoginTimeout + time.Minute + 30*time.Second,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func RemoveUserOnSigninScreen(ctx context.Context, s *testing.State) {
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	signinVar := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	if err := userutil.ResetUsers(ctx); err != nil {
		s.Fatal("Failed to clear state: ", err)
	}

	// Setup three users with the same password.
	firstUser := "first-user@gmail.com"
	secondUser := "second-user@gmail.com"
	thirdUser := "third-user@gmail.com"
	password := "password"
	for _, user := range []string{firstUser, secondUser, thirdUser} {
		if err := userutil.CreateUser(ctx, user, password, chrome.KeepState()); err != nil {
			s.Fatal("Failed to create user: ", err)
		}
	}
	defer userutil.ResetUsers(ctx)

	// Go to the login screen, remove second user and check that second user pod is gone.
	func() {
		// We need NoLogin() to get on the login screen, and we need
		// --skip-force-online-signin-for-testing so that we're not asked to sign in when focussing a
		// user pod.
		cr, err := chrome.New(
			ctx,
			chrome.NoLogin(),
			chrome.KeepState(),
			chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
			chrome.LoadSigninProfileExtension(signinVar),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanUpCtx)

		tconn, err := cr.SigninProfileTestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(cleanUpCtx, s.OutDir(), s.HasError, tconn)
		ui := uiauto.New(tconn)

		clickSecondUser := uiauto.Combine(
			"click on second user to focus",
			// Focus second user pod as ui.RemoveUseronLoginScreen requirs that.
			ui.LeftClick(nodewith.Name(secondUser).Role(role.Button)),
		)

		// Second user need to be brought to focus.
		if err := clickSecondUser(ctx); err != nil {
			s.Fatal("Failed to click on second user to bring it to focus: ", err)
		}

		if err := userutil.RemoveUserOnLoginScreen(ctx, tconn, cr, secondUser); err != nil {
			s.Fatal("Failed to remove user on login screen: ", err)
		}

		// Check that cryptohome has gone.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := getCryptohomeInfo(ctx, secondUser); err == nil {
				return errors.New("Cryptohome directory still exists")
			} else if os.IsNotExist(err) {
				return nil
			} else {
				return testing.PollBreak(errors.Wrap(err, "unexpected error"))
			}
		}, &testing.PollOptions{
			Timeout:  20 * time.Second,
			Interval: time.Second,
		}); err != nil {
			s.Fatalf("Removal of user %s failed: %v", secondUser, err)
		}

		// Check that second user is gone.
		if err := ui.WaitUntilGone(nodewith.Name(secondUser))(ctx); err != nil {
			s.Fatal("Second user pod has not disappeared: ", err)
		}

		// Check that the local state is gone as well.
		knownEmails, err := userutil.GetKnownEmailsFromLocalState()
		if err != nil {
			s.Fatal("Failed to get known emails from local state: ", err)
		}
		if knownEmails[secondUser] {
			s.Fatal("Removed user is still in LoggedInUsers list")
		}
	}()

	// Restart chrome and check that the user we just removed is still gone.
	func() {
		cr, err := chrome.New(
			ctx,
			chrome.NoLogin(),
			chrome.KeepState(),
			chrome.LoadSigninProfileExtension(signinVar),
		)
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanUpCtx)

		tconn, err := cr.SigninProfileTestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}
		defer faillog.DumpUITreeOnError(cleanUpCtx, s.OutDir(), s.HasError, tconn)
		ui := uiauto.New(tconn)

		// Wait until we find reference to firstUser.
		if err := ui.WaitUntilExists(nodewith.Name(firstUser).First())(ctx); err != nil {
			s.Fatal("Failed to wait for user pod to be available after restart: ", err)
		}

		// Check that we cannot find reference to second user.
		if err := ui.EnsureGoneFor(nodewith.Name(secondUser), 10*time.Second)(ctx); err != nil {
			s.Fatal("Failed to ensure that second user is gone: ", err)
		}
	}()
}

func getCryptohomeInfo(ctx context.Context, user string) (os.FileInfo, error) {
	path, err := cryptohome.UserPath(ctx, user)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get path to %s's cryptohome", user)
	}

	return os.Stat(path)
}
