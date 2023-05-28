// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CryptohomeRecovery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks cryptohome account recovery flow",
		Contacts: []string{
			"cros-lurs@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},
		Timeout: 2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
		Fixture: fixture.CleanOwnership,
	})
}

func CryptohomeRecovery(ctx context.Context, s *testing.State) {
	const (
		testFile = "test_file"
		testData = "test that data persisted after the recovery"
	)

	var creds chrome.Creds

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	cryptohome := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	// Enable the UserSecretStash experiment.
	cleanupUSSExperiment, err := helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	defer cleanupUSSExperiment(cleanupCtx)

	// Log in and log out to create a user pod on the login screen.
	func() {
		cr, err := chrome.New(ctx,
			chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			chrome.DontSkipOOBEAfterLogin(),
			chrome.EnableFeatures("CryptohomeRecovery"),
		)
		if err != nil {
			s.Fatal("Chrome login failed: ", err)
		}
		defer cr.Close(cleanupCtx)
		creds = cr.Creds()

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}
		defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to create OOBE connection: ", err)
		}
		defer oobeConn.Close()

		if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('recovery-check')", nil); err != nil {
			s.Fatal("Failed to advance to the 'recovery-check' screen: ", err)
		}
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.isReadyForTesting()"); err != nil {
			s.Fatal("Failed to wait for the consolidated consent screen to be visible: ", err)
		}
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.enableRecoveryToggle()", nil); err != nil {
			s.Fatal("Failed to enable recovery toggle on the consolidated consent screen: ", err)
		}
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ConsolidatedConsentScreen.clickAcceptButton()", nil); err != nil {
			s.Fatal("Failed to click consolidated consent screen accept button: ", err)
		}

		if err := waitForRecoverySetup(ctx, oobeConn); err != nil {
			s.Fatal("Failed to wait for recovery setup to be finished: ", err)
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.skipPostLoginScreens()", nil); err != nil {
			// This is not fatal because sometimes it fails because Oobe shutdowns too fast after the call - which produces error.
			s.Log("Failed to call skip post login screens: ", err)
		}
		if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
			s.Fatal("Failed to wait for OOBE to be dismissed: ", err)
		}

		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}

		// Write test file to check that data persisted on password change.
		if err := hwsec.WriteUserTestContent(ctx, cryptohome, cmdRunner, cr.NormalizedUser(), testFile, testData); err != nil {
			s.Fatal("Failed to write a user test file: ", err)
		}

		s.Log("The user was created - logging out")
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	// chrome.KeepState() is needed to show the login screen with a user pod
	// (instead of the OOBE login screen).
	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(creds),
		chrome.DeferLogin(),
		chrome.ReauthMode(),
		chrome.KeepState(),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.EnableFeatures("CryptohomeRecovery"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on the login screen: ", err)
	}
	defer cr.Close(cleanupCtx)

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tLoginConn)

	if err := enterInvalidPassword(ctx, tLoginConn, creds); err != nil {
		s.Fatal("Failed to enter invalid password: ", err)
	}

	s.Log("Starting recovery flow")
	if err := lockscreen.ClickRecoverUser(ctx, tLoginConn); err != nil {
		s.Fatal("Failed to click recovery button: ", err)
	}
	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Chrome login during recovery failed: ", err)
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if err := lockscreen.WaitForLoggedIn(ctx, tLoginConn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login after recovery: ", err)
	}
	if err := ash.WaitForShelf(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Shelf did not appear after logging in: ", err)
	}

	// Read test file to check that data persisted on password change.
	if content, err := hwsec.ReadUserTestContent(ctx, cryptohome, cmdRunner, cr.NormalizedUser(), testFile); err != nil {
		s.Fatal("Failed to read a user test file: ", err)
	} else if !bytes.Equal(content, []byte(testData)) {
		s.Fatalf("Unexpected test file content: got %q, want %q", content, testData)
	}
}

// waitForRecoverySetup navigates to the recovery setup screen and waits until
// it's not shown anymore.
func waitForRecoverySetup(ctx context.Context, oobeConn *chrome.Conn) error {
	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('cryptohome-recovery-setup')", nil); err != nil {
		return errors.Wrap(err, "failed to advance to the TOS screen")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.CryptohomeRecoverySetupScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the recovery setup screen to be visible")
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "!OobeAPI.screens.CryptohomeRecoverySetupScreen.isVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the recovery setup screen to not be visible")
	}

	return nil
}

// enterInvalidPassword enters invalid password on the login screen and waits until
// the auth error dialog is shown.
func enterInvalidPassword(ctx context.Context, tconn *chrome.TestConn, creds chrome.Creds) error {
	// Wait for the login screen to be ready for password entry.
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.ReadyForPassword }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "failed waiting for the login screen to be ready for password entry last state %+v", st)
	}

	// TODO(crbug/1109381): the password field isn't actually ready just yet when WaitState returns.
	// This causes it to miss some of the keyboard input, so the password will be wrong.
	// We can check in the UI for the password field to exist, which seems to be a good enough indicator that
	// the field is ready for keyboard input.
	if err := lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 5*time.Second); err != nil {
		return errors.Wrap(err, "password text field did not appear in the UI")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get virtual keyboard")
	}
	defer keyboard.Close(ctx)

	// Enter wrong password.
	if err = lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass+"invalid", keyboard); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}
	if err := lockscreen.WaitForAuthError(ctx, tconn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for auth error")
	}

	return nil
}
