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
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
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
			"anastasiian@chromium.org",
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
			"ui.gaiaPoolDefault",
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.GAIALoginTimeout + chrome.LoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Credentials sync - successful password change.
			Value: "screenplay-1b766b3e-874a-49dd-be9d-5c63994970e3",
		}},
		Params: []testing.Param{{
			Name: "no_consumer_update",
			Val:  false,
		}, {
			Name: "consumer_update",
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

	// TODO(b/286348339): use a smaller timeout value after b/286348339 is resolved.
	if err := ping.VerifyInternetConnectivity(ctx, 20*time.Second); err != nil {
		// Only printing log instead of report error here, to avoid lab network issue causing test flakiness.
		testing.ContextLog(ctx, "Failed to verify Internet connectivity before test: ", err)
	}

	// Isolate the step to leverage `defer` pattern.
	func() {
		var err error
		gaiaCreds, err = credconfig.PickRandomCreds(s.RequiredVar("ui.gaiaPoolDefault"))
		if err != nil {
			s.Fatal("Failed to parse creds: ", err)
		}

		initialCreds = gaiaCreds
		// Add a whitespace to the password, so when user logs in again - password change would be detected.
		// Note: the password with a whitespace will still be accepted by Gaia.
		initialCreds.Pass = " " + initialCreds.Pass
		cr, err := chrome.New(
			ctx, chrome.GAIALogin(initialCreds))
		if err != nil {
			s.Fatal("Failed to create a user: ", err)
		}
		defer cr.Close(ctx)
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
		isConsumerUpdate := s.Param().(bool)

		options := []chrome.Option{
			chrome.GAIALogin(gaiaCreds),
			chrome.DontWaitForCryptohome(),
			chrome.KeepState(),
			chrome.RemoveNotification(false), // By default it waits for the user session.
			chrome.DontSkipOOBEAfterLogin(),
		}

		if isConsumerUpdate {
			options = append(options, chrome.ExtraArgs("--enable-features=OobeSoftwareUpdate"))
		}

		cr, err := chrome.New(ctx, options...)

		if err != nil {
			s.Fatal("Chrome login failed: ", err)
		}
		defer cr.Close(ctx)
		oobeConn, err := cr.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to wait for OOBE connection: ", err)
		}
		defer oobeConn.Close()

		if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "!document.querySelector('#gaia-password-changed').hidden", 10*time.Second); err != nil {
			s.Fatal("Failed to wait for the gaia password changed screen: ", err)
		}
		if err := oobeConn.Eval(ctx, fmt.Sprintf("document.querySelector('#gaia-password-changed').$.oldPasswordInput.value = '%s'", initialCreds.Pass), nil); err != nil {
			s.Fatal("Failed to enter old password: ", err)
		}
		if err := oobeConn.Eval(ctx, "document.querySelector('#gaia-password-changed').$.next.click()", nil); err != nil {
			s.Fatal("Failed to click on next button: ", err)
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
	defer cr.Close(ctx)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting signing test API connection failed: ", err)
	}

	if err = lockscreen.WaitForPasswordField(ctx, tconn, gaiaCreds.User, 10*time.Second); err != nil {
		s.Fatal("Fail to wait for password: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(ctx)
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
