// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type passwordType int

const (
	gaiaPassword passwordType = iota
	localPassword
	pinOnly
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ExistingUser,
		Desc: "Checks that an existing device user can login from the login screen",
		Contacts: []string{
			"cros-lurs@google.com",
			"rrsilva@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia", "non_meet_device"},
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute,
		Params: []testing.Param{
			{
				Val: gaiaPassword,
			}, {
				Name:      "local",
				Val:       localPassword,
				ExtraAttr: []string{"informational"},
			}, {
				Name:      "pin",
				Val:       pinOnly,
				ExtraAttr: []string{"informational"},
			},
		},
	})
}

// ExistingUser logs in to an existing user account from the login screen.
func ExistingUser(ctx context.Context, s *testing.State) {
	const (
		localPass = "testpass"
		pin       = "123456"
	)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	authFactor := s.Param().(passwordType)
	var creds chrome.Creds
	// Ensure that as the test ends, we cleanup any state.
	defer userutil.ResetUsers(cleanupContext)

	// Log in and log out to create a user pod on the login screen.
	func() {
		var cr *chrome.Chrome
		switch authFactor {
		case gaiaPassword:
			cr, creds = logInWithGaiaPassword(ctx, s)
		case localPassword:
			cr, creds = logInWithLocalPassword(ctx, s, localPass)
		case pinOnly:
			cr, creds = logInWithPin(ctx, s, pin)
		}
		defer cr.Close(ctx)

		// This is needed for reven tests, as login flow there relies on the existence of a device setting.
		if err := userutil.WaitForOwnership(ctx, cr); err != nil {
			s.Fatal("User did not become device owner: ", err)
		}
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	// chrome.NoLogin() and chrome.KeepState() are needed to show the login
	// screen with a user pod (instead of the OOBE login screen).
	opts := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupContext)

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_dump")

	// Wait for the login screen to be ready for password entry.
	if err := lockscreen.WaitForPasswordEntry(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Failed waiting for the login screen to be ready for password entry: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupContext)

	if authFactor != pinOnly {
		s.Log("Entering password to log in")
		if err := lockscreen.EnterPassword(ctx, tLoginConn, creds.User, creds.Pass, kb); err != nil {
			s.Fatal("Failed to enter password: ", err)
		}
	} else {
		s.Log("Entering pin to log in")
		if err := lockscreen.EnterPIN(ctx, tLoginConn, kb, pin); err != nil {
			s.Fatal("Failed to enter pin: ", err)
		}
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if err := lockscreen.WaitForLoggedIn(ctx, tLoginConn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	if err := ash.WaitForShelf(ctx, tLoginConn, 30*time.Second); err != nil {
		s.Fatal("Shelf did not appear after logging in: ", err)
	}
}

// logInWithGaiaPassword logs the real user in and returns the credentials that
// can be user for the offline login afterwards.
func logInWithGaiaPassword(ctx context.Context, s *testing.State) (c *chrome.Chrome, creds chrome.Creds) {
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)))
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	return cr, cr.Creds()
}

// logInWithLocalPassword logs the fake user in with local password and returns
// the credentials that can be user for the offline login afterwards.
func logInWithLocalPassword(ctx context.Context, s *testing.State, localPassword string) (c *chrome.Chrome, creds chrome.Creds) {
	cr, err := login.SetupUserWithLocalPassword(ctx,
		localPassword,
		chrome.FakeLogin(chrome.Creds{User: "testuser@gmail.com", Pass: ""}),
	)
	if err != nil {
		s.Fatal("Failed to setup user: ", err)
	}

	return cr, chrome.Creds{User: cr.Creds().User, Pass: localPassword}
}

// logInWithPin logs the fake user in with pin and returns
// the credentials that can be user for the offline login afterwards.
func logInWithPin(ctx context.Context, s *testing.State, pin string) (c *chrome.Chrome, creds chrome.Creds) {
	cr, err := login.SetupUserWithPin(ctx,
		pin,
		chrome.FakeLogin(chrome.Creds{User: "testuser@gmail.com", Pass: ""}),
	)
	if err != nil {
		s.Fatal("Failed to setup user: ", err)
	}

	return cr, chrome.Creds{User: cr.Creds().User, Pass: ""}
}
