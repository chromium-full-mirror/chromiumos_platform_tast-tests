// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type logoutMethod int

const (
	logoutShortcut logoutMethod = iota
	logoutClickOnButton
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Logout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the sign in page shows after signing out",
		Contacts: []string{
			"cros-oac@google.com",
			"cros-lurs@google.com",
			"awendy@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      chrome.LoginTimeout + time.Minute,
		Params: []testing.Param{
			{
				Val: logoutShortcut,
			}, {
				Name: "button",
				Val:  logoutClickOnButton,
			},
		},
	})
}

// Logout logs out and verifies if the sign in screen is being displayed.
func Logout(ctx context.Context, s *testing.State) {
	method := s.Param().(logoutMethod)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	const (
		username = "testuser@gmail.com"
		password = "testpass"
	)

	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if cr != nil {
			cr.Close(cleanupCtx)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer func() {
		faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "logout")
	}()

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	const state = "stopped"
	sw, err := sm.WatchSessionStateChanged(ctx, state)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	switch method {
	case logoutShortcut:
		if err := logoutByShortcut(ctx, tconn); err != nil {
			s.Fatal("Failed to logout: ", err)
		}
	case logoutClickOnButton:
		if err := quicksettings.SignOut(ctx, tconn); err != nil {
			s.Fatal("Failed to logout: ", err)
		}
	}

	s.Logf("Waiting for SessionStateChanged %q D-Bus signal from session_manager", state)
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal")
	case <-ctx.Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", ctx.Err())
	}

	// Close Chrome to do some cleanup.
	// It will log some errors, as the session is closed already.
	cr.Close(ctx)
	cr = nil

	if cr, err = chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	); err != nil {
		s.Fatal("Failed to restart Chrome for testing: ", err)
	}

	if tconn, err = cr.SigninProfileTestAPIConn(ctx); err != nil {
		s.Fatal("Failed to re-establish test API connection")
	}

	if err := lockscreen.WaitForPasswordEntry(ctx, tconn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for login screen: ", err)
	}

	if err := lockscreen.EnterPassword(ctx, tconn, username, password, kb); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if _, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.LoggedIn }, 10*time.Second); err != nil {
		s.Fatal("Failed to log in: ", err)
	}
}

func logoutByShortcut(ctx context.Context, tconn *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create the keyboard")
	}
	defer kb.Close(ctx)

	return uiauto.Combine("press the keyboard to logout",
		kb.AccelAction("Shift+Ctrl+Q"),
		kb.AccelAction("Shift+Ctrl+Q"),
	)(ctx)
}
