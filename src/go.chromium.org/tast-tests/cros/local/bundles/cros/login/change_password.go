// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/signinutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	testFile = "test_file"
	testData = "test that data persisted on the password change"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangePassword,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks cryptohome password change flow",
		Contacts: []string{
			"cros-lurs@google.com",
			"rrsilva@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{"group:mainline", "group:hw_agnostic"},
		VarDeps: []string{
			ui.GaiaPoolDefaultVarName,
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.GAIALoginTimeout + chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Credentials sync - successful password change.
			Value: "screenplay-1b766b3e-874a-49dd-be9d-5c63994970e3",
		}},
		Params: []testing.Param{{
			Name: "user_without_recovery",
			Val:  false,
		}, {
			Name: "user_with_recovery",
			Val:  true,
		}},
	})
}

func ChangePassword(ctx context.Context, s *testing.State) {
	var initialCreds chrome.Creds
	var gaiaCreds chrome.Creds
	var normalizedUser string

	cmdRunner := hwseclocal.NewCmdRunner()
	cryptohome := hwsec.NewCryptohomeClient(cmdRunner)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupCtx)

	if err := ping.VerifyInternetConnectivity(ctx, 10*time.Second); err != nil {
		// Only printing log instead of report error here, to avoid lab network issue causing test flakiness.
		testing.ContextLog(ctx, "Failed to verify Internet connectivity before test: ", err)
	}

	userHasRecovery := s.Param().(bool)

	// Isolate the step to leverage `defer` pattern.
	func() {
		var err error
		gaiaCreds, err = credconfig.PickRandomCreds(dma.CredsFromPool(ui.GaiaPoolDefaultVarName))
		if err != nil {
			s.Fatal("Failed to parse creds: ", err)
		}

		initialCreds = gaiaCreds
		// Add a whitespace to the password, so when user logs in again - password change would be detected.
		// Note: the password with a whitespace will still be accepted by Gaia.
		initialCreds.Pass = " " + initialCreds.Pass

		options := []chrome.Option{
			chrome.GAIALogin(initialCreds),
		}
		// We don't have a way to configure recovery via chrome.New params, so
		// rely on Feature flag for now to control recovery factor.
		if userHasRecovery {
			options = append(options, chrome.EnableFeatures("CryptohomeRecoveryByDefaultForConsumers"))
		} else {
			options = append(options, chrome.DisableFeatures("CryptohomeRecoveryByDefaultForConsumers"))
		}

		cr, err := chrome.New(ctx, options...)
		if err != nil {
			s.Fatal("Failed to create a user: ", err)
		}
		defer cr.Close(cleanupCtx)
		normalizedUser = cr.NormalizedUser()
		if err := hwsec.WriteUserTestContent(ctx, cryptohome, cmdRunner, normalizedUser, testFile, testData); err != nil {
			s.Fatal("Failed to write a user test file: ", err)
		}
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
			chrome.DeferLogin(),
			chrome.DontWaitForCryptohome(),
			chrome.ReauthMode(),
			chrome.KeepState(),
			chrome.DontSkipOOBEAfterLogin(),
			chrome.RemoveNotification(false), // By default it waits for the user session.
			chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
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

		if err := signinutil.EnterInvalidPassword(ctx, cr, gaiaCreds); err != nil {
			s.Fatal("Failed to enter invalid password: ", err)
		}

		s.Log("Starting reauth flow")
		if err := lockscreen.ClickRecoverUser(ctx, tLoginConn); err != nil {
			s.Fatal("Failed to click recovery button: ", err)
		}

		if err := cr.ContinueLogin(ctx); err != nil {
			s.Fatal("Chrome login during recovery failed: ", err)
		}

		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to wait for OOBE connection: ", err)
		}
		defer oobeConn.Close()

		if !userHasRecovery {
			// Without recovery user need to enter old password.
			if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "!document.querySelector('#enter-old-password').hidden", 45*time.Second); err != nil {
				s.Fatal("Failed to wait for enter old password screen: ", err)
			}

			if err := oobeConn.Eval(ctx, fmt.Sprintf("document.querySelector('#enter-old-password').$.oldPasswordInput.value = '%s'", initialCreds.Pass), nil); err != nil {
				s.Fatal("Failed to enter the old password: ", err)
			}
			if err := oobeConn.Eval(ctx, "document.querySelector('#enter-old-password').$.next.click()", nil); err != nil {
				s.Fatal("Failed to click on the next button: ", err)
			}
		}

		if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isVisible()", 45*time.Second); err != nil {
			s.Fatal("Failed to wait for factor setup success screen: ", err)
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
			s.Fatal("Failed to click on the done button: ", err)
		}

		if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
			s.Fatal("Failed to wait for OOBE to be dismissed: ", err)
		}

		// Read test file to check that data persisted on password change.
		if content, err := hwsec.ReadUserTestContent(ctx, cryptohome, cmdRunner, normalizedUser, testFile); err != nil {
			s.Fatal("Failed to read a user test file: ", err)
		} else if !bytes.Equal(content, []byte(testData)) {
			s.Fatalf("Unexpected test file content: got %q, want %q", content, testData)
		}
	}()

	// Login again with the updated password.
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

	if err = lockscreen.WaitForPasswordField(ctx, tconn, gaiaCreds.User, 25*time.Second); err != nil {
		s.Fatal("Fail to wait for password: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)
	if err = lockscreen.EnterPassword(ctx, tconn, gaiaCreds.User, gaiaCreds.Pass, keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if err := lockscreen.WaitForLoggedIn(ctx, tconn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	// Read test file to check that data persisted after login with updated password.
	if content, err := hwsec.ReadUserTestContent(ctx, cryptohome, cmdRunner, normalizedUser, testFile); err != nil {
		s.Fatal("Failed to read a user test file: ", err)
	} else if !bytes.Equal(content, []byte(testData)) {
		s.Fatalf("Unexpected test file content: got %q, want %q", content, testData)
	}
}
