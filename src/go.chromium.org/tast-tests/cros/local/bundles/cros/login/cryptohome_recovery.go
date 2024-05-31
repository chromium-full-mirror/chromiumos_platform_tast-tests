// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/signinutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CryptohomeRecovery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks cryptohome account recovery flow",
		Contacts: []string{
			"cros-lurs@google.com",
			"antrim@chromium.org",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
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

	// Log in and log out to create a user pod on the login screen.
	func() {
		cr, err := chrome.New(ctx,
			chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
			chrome.DontSkipOOBEAfterLogin(),
			chrome.EnableFeatures("CryptohomeRecovery"),
			// TODO(b/315829727): Remove this as a part of post-launch cleanup.
			chrome.EnableFeatures("LocalPasswordsForConsumers"),
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

		if err := signinutil.WaitForRecoverySetup(ctx, oobeConn); err != nil {
			s.Fatal("Failed to wait for recovery setup to be finished: ", err)
		}

		s.Log("Waiting for the password selection screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.PasswordSelectionScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the password selection screen to be visible: ", err)
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordSelectionScreen.selectGaiaPassword()", nil); err != nil {
			s.Fatal("Failed to select GAIA password: ", err)
		}

		ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
		nextButton := nodewith.Name("Next").Role(role.Button)
		if err := uiauto.Combine("click next on the password selection screen",
			ui.WaitUntilEnabled(nextButton),
			ui.LeftClick(nextButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click password selection screen next button: ", err)
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
		chrome.EnableFeatures("CryptohomeRecoveryBeforeFlowSplit"),
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

	if err := signinutil.EnterInvalidPassword(ctx, cr, creds); err != nil {
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
