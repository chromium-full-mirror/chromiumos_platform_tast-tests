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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/auth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type setupWithAuthFactorType int

const (
	setupWithGaiaPassword setupWithAuthFactorType = iota
	setupWithLocalPassword
	setupWithPin
)

type changePasswordParams struct {
	// Specify if the user has recovery enabled by default during setup.
	userHasRecovery bool
	// Specify if the user should setup recovery in settings.
	setUpRecovery bool
	// Setup user with local password
	authFactorType setupWithAuthFactorType
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ChangePassword,
		Desc: "Checks cryptohome password change flow",
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
			"gaia",
			"pinweaver",
			"tpm2",
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
			Val: changePasswordParams{
				userHasRecovery: false,
				setUpRecovery:   false,
				authFactorType:  setupWithGaiaPassword,
			},
			ExtraAttr: []string{"informational"},
		}, {
			Name: "user_without_recovery_and_setup",
			Val: changePasswordParams{
				userHasRecovery: false,
				setUpRecovery:   true,
				authFactorType:  setupWithGaiaPassword,
			},
			ExtraAttr: []string{"informational"},
		}, {
			Name: "user_with_recovery_gaia",
			Val: changePasswordParams{
				userHasRecovery: true,
				setUpRecovery:   false,
				authFactorType:  setupWithGaiaPassword,
			},
			ExtraAttr: []string{"informational"},
		}, {
			Name: "user_with_recovery_local_password",
			Val: changePasswordParams{
				userHasRecovery: true,
				setUpRecovery:   true,
				authFactorType:  setupWithLocalPassword,
			},
			ExtraAttr: []string{"informational"},
		}, {
			Name: "user_with_recovery_pin",
			Val: changePasswordParams{
				userHasRecovery: true,
				setUpRecovery:   true,
				authFactorType:  setupWithPin,
			},
			ExtraAttr: []string{"informational"},
		}},
	})
}

func ChangePassword(ctx context.Context, s *testing.State) {
	var initialCreds chrome.Creds
	var gaiaCreds chrome.Creds
	var normalizedUser string

	const (
		testFile         = "test_file"
		testData         = "test that data persisted on the password change"
		localPassword    = "local_password"
		localPasswordNew = "local_password_new"
		pin              = "123456"
		pinNew           = "654321"
	)

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

	// Make sure that we actually reset users to avoid flakiness due to previous tests.
	userutil.ResetUsers(ctx)
	userParam := s.Param().(changePasswordParams)

	var options []chrome.Option
	// We don't have a way to configure recovery via chrome.New params, so
	// rely on Feature flag for now to control recovery factor.
	if userParam.userHasRecovery {
		options = append(options, chrome.EnableFeatures("CryptohomeRecoveryByDefaultForConsumers"))
	} else {
		options = append(options, chrome.DisableFeatures("CryptohomeRecoveryByDefaultForConsumers"))
	}

	gaiaCreds, err := credconfig.PickRandomCreds(dma.CredsFromPool(ui.GaiaPoolDefaultVarName))
	if err != nil {
		s.Fatal("Failed to parse creds: ", err)
	}

	// Isolate the step to leverage `defer` pattern.
	func() {
		var err error
		var cr *chrome.Chrome
		switch userParam.authFactorType {
		case setupWithGaiaPassword:
			{
				initialCreds = gaiaCreds
				// Add a whitespace to the password, so when user logs in again - password change would be detected.
				// Note: the password with a whitespace will still be accepted by Gaia.
				initialCreds.Pass = " " + initialCreds.Pass
				options = append(options, chrome.GAIALogin(initialCreds))
				cr, err = chrome.New(ctx, options...)
			}
		case setupWithLocalPassword:
			{
				options = append(options, chrome.GAIALogin(gaiaCreds))
				cr, err = login.SetupUserWithLocalPassword(ctx, localPassword, options...)
			}
		case setupWithPin:
			{
				options = append(options, chrome.GAIALogin(gaiaCreds))
				s.Logf("User: %s , Pass: %s", gaiaCreds.User, gaiaCreds.Pass)
				cr, err = login.SetupUserWithPin(ctx, pin, options...)
			}

		}
		if err != nil {
			s.Fatal("Failed to create a user: ", err)
		}
		defer cr.Close(cleanupCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "first_login")

		normalizedUser = cr.NormalizedUser()
		if err := hwsec.WriteUserTestContent(ctx, cryptohome, cmdRunner, normalizedUser, testFile, testData); err != nil {
			s.Fatal("Failed to write a user test file: ", err)
		}
		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}

		if userParam.setUpRecovery {
			// GoBigSleepLint, there is a launcher showing up due to real gaia sign in.
			testing.Sleep(ctx, time.Second*10)

			// Set up Recovery through a connection to the Settings page.
			settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
			if err != nil {
				s.Fatal("Failed to open setting page: ", err)
			}
			defer settings.Close(ctx)
			switch userParam.authFactorType {
			case setupWithGaiaPassword:
				{
					if err := auth.ConfirmPassword(ctx, cr, initialCreds.Pass); err != nil {
						s.Fatal("Failed to confirm password: ", err)
					}

				}
			case setupWithLocalPassword:
				{
					if err := auth.ConfirmPassword(ctx, cr, localPassword); err != nil {
						s.Fatal("Failed to confirm password: ", err)
					}
				}
			case setupWithPin:
				{
					if err := auth.ConfirmPin(ctx, cr, pin, true); err != nil {
						s.Fatal("Failed to confirm password: ", err)
					}
				}
			}

			if err := settings.SetToggleOption(cr, "Local data recovery", true)(ctx); err != nil {
				s.Fatal("Failed to toggle recovery: ", err)
			}
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
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

		if userParam.authFactorType != setupWithPin {
			if err := signinutil.EnterInvalidPassword(ctx, cr, gaiaCreds); err != nil {
				s.Fatal("Failed to enter invalid password: ", err)
			}
		} else {
			if err := signinutil.EnterInvalidPin(ctx, cr, pinNew); err != nil {
				s.Fatal("Failed to enter pin: ", err)
			}
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

		if !userParam.userHasRecovery && !userParam.setUpRecovery {
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

			if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isDone()"); err != nil {
				s.Fatal("Failed to see password success screen")
			}

			if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
				s.Fatal("Failed to click done")
			}
		} else {
			switch userParam.authFactorType {
			case setupWithGaiaPassword:
				{
					if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isVisible()", 45*time.Second); err != nil {
						s.Fatal("Failed to wait for factor setup success screen: ", err)
					}

					if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
						s.Fatal("Failed to click on the done button: ", err)
					}
				}
			case setupWithLocalPassword:
				{
					if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.isReadyForTesting()"); err != nil {
						s.Fatal("Failed to get local password setup screen")
					}
					if err := oobeConn.Call(ctx, nil, `(pw) => { OobeAPI.screens.LocalPasswordSetupScreen.enterPasswordToFirstInput(pw); }`, localPasswordNew); err != nil {
						s.Fatal("Failed to enter first password")
					}
					if err := oobeConn.Call(ctx, nil, `(pw) => { OobeAPI.screens.LocalPasswordSetupScreen.enterPasswordToConfirmInput(pw); }`, localPasswordNew); err != nil {
						s.Fatal("Failed to enter second password")
					}
					if err := oobeConn.WaitForExprWithTimeout(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.nextButton.isEnabled()", 3*time.Second); err != nil {
						s.Fatal("Failed to see next button enabled")
					}
					if err := oobeConn.Eval(ctx, "OobeAPI.screens.LocalPasswordSetupScreen.nextButton.click()", nil); err != nil {
						s.Fatal("Failed to click next button")
					}
					if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.isDone()"); err != nil {
						s.Fatal("Failed to see password success screen")
					}
					if err := oobeConn.Eval(ctx, "OobeAPI.screens.PasswordFactorSuccessScreen.clickDone()", nil); err != nil {
						s.Fatal("Failed to click done")
					}
				}
			case setupWithPin:
				{
					if err := oobeConn.WaitForExprFailOnErr(ctx, "!document.querySelector('#pin-setup').hidden"); err != nil {
						s.Fatal("Failed to see pin setup screen")
					}

					for _, step := range []string{"start", "confirm"} {
						if err := oobeConn.WaitForExprFailOnErr(ctx, fmt.Sprintf("document.querySelector('#pin-setup').uiStep === '%s'", step)); err != nil {
							s.Fatal("Failed to see pin setup screen")
						}
						if err := oobeConn.Eval(ctx, fmt.Sprintf("document.querySelector('#pin-setup').$.pinKeyboard.$.pinKeyboard.$.pinInput.value = '%s'", pinNew), nil); err != nil {
							s.Fatal("Failed to enter pin")
						}

						if err := oobeConn.Eval(ctx, "document.querySelector('#pin-setup').$.nextButton.click()", nil); err != nil {
							s.Fatal("Failed to click next button")
						}
					}

					if err := oobeConn.WaitForExprFailOnErr(ctx, "document.querySelector('#pin-setup').uiStep === 'done'"); err != nil {
						s.Fatal("Failed to setup new pin")
					}

					if err := oobeConn.Eval(ctx, "document.querySelector('#pin-setup').$.doneButton.click()", nil); err != nil {
						s.Fatal("Failed to setup new pin")
					}
				}
			}
		}
	}()

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

	switch userParam.authFactorType {
	case setupWithGaiaPassword:
		{
			// Verify we can not login with the old password.
			if err := signinutil.EnterInvalidPassword(ctx, cr, initialCreds); err != nil {
				s.Fatal("Failed to verify old password does not work: ", err)
			}

			// Verify we can login with the new password.
			if err := signinutil.EnterValidPassword(ctx, cr, gaiaCreds); err != nil {
				s.Fatal("Failed to verify new password does work: ", err)
			}

			// Verify we can verify file contents.
			if err := checkFileContents(ctx, cleanupCtx, s, normalizedUser, testFile, testData); err != nil {
				s.Fatal("Failed to verify test file data persisted: ", err)
			}
		}
	case setupWithLocalPassword:
		{
			creds := chrome.Creds{User: initialCreds.User, Pass: localPassword}
			// Verify we can not login with the old password.
			if err := signinutil.EnterInvalidPassword(ctx, cr, creds); err != nil {
				s.Fatal("Failed to verify old password does not work: ", err)
			}

			// Verify we can login with the new password.
			creds.Pass = localPasswordNew
			if err := signinutil.EnterValidPassword(ctx, cr, creds); err != nil {
				s.Fatal("Failed to verify new password does work: ", err)
			}

			// Verify we can verify file contents.
			if err := checkFileContents(ctx, cleanupCtx, s, normalizedUser, testFile, testData); err != nil {
				s.Fatal("Failed to verify test file data persisted: ", err)
			}
		}
	case setupWithPin:
		{
			if err := signinutil.EnterInvalidPin(ctx, cr, pin); err != nil {
				s.Fatal("Failed to verify olf pin does not work: ", err)
			}

			if err := signinutil.EnterValidPin(ctx, cr, pinNew); err != nil {
				s.Fatal("Failed to verify old pin does not work: ", err)
			}

			// Verify we can login with the new pin.
			if err := checkFileContents(ctx, cleanupCtx, s, normalizedUser, testFile, testData); err != nil {
				s.Fatal("Failed to verify test file data persisted: ", err)
			}
		}

	}
}

func checkFileContents(ctx, cleanupCtx context.Context, s *testing.State, normalizedUser, testFile, testData string) error {
	cmdRunner := hwseclocal.NewCmdRunner()
	cryptohome := hwsec.NewCryptohomeClient(cmdRunner)

	// Read test file to check that data persisted after login with updated password.
	if content, err := hwsec.ReadUserTestContent(ctx, cryptohome, cmdRunner, normalizedUser, testFile); err != nil {
		return errors.Wrap(err, "failed to read user test file")
	} else if !bytes.Equal(content, []byte(testData)) {
		return errors.Wrapf(err, "unexpected test file content: got %q, want %q", content, testData)
	}
	return nil
}
