// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/signinutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CryptohomeRecoveryFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks cryptohome password change flow, with setting up the flow from OS Settings",
		Contacts: []string{
			"cros-lurs@google.com",
			"antrim@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{"group:mainline", "informational", "group:hw_agnostic"},
		VarDeps: []string{
			ui.GaiaPoolDefaultVarName,
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: chrome.GAIALoginTimeout + 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
	})
}

func CryptohomeRecoveryFromSettings(ctx context.Context, s *testing.State) {
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
			chrome.GAIALoginPool(s.RequiredVar(ui.GaiaPoolDefaultVarName)),
			chrome.EnableFeatures("CryptohomeRecovery"),
			chrome.EnableFeatures("CryptohomeRecoveryBeforeFlowSplit"),
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
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "first_login")

		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}

		// Write test file to check that data persisted on password change.
		if err := hwsec.WriteUserTestContent(ctx, cryptohome, cmdRunner, cr.NormalizedUser(), testFile, testData); err != nil {
			s.Fatal("Failed to write a user test file: ", err)
		}

		// Set up Recovery through a connection to the Settings page.
		settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
		if err != nil {
			s.Fatal("Failed to open setting page: ", err)
		}
		defer settings.Close(ctx)

		if err := ossettings.ConfirmPassword(ctx, cr, creds.Pass); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}

		if err := settings.SetToggleOption(cr, "Local data recovery", true)(ctx); err != nil {
			s.Fatal("Failed to toggle recovery: ", err)
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "recovery")

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
