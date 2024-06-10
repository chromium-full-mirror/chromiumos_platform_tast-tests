// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type openSettingsParam struct {
	// Set to true to have the test type in the password, false to cancel instead.
	usePassword bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open OS Settings and access them with a password",
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{"group:mainline", "informational", "group:hw_agnostic"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute,
		Params: []testing.Param{{
			Name: "with_password",
			Val: openSettingsParam{
				usePassword: true,
			},
		}, {
			Name: "cancel",
			Val: openSettingsParam{
				usePassword: false,
			},
		}},
	})
}

func OpenSettings(ctx context.Context, s *testing.State) {
	const (
		username = "testuser@gmail.com"
		password = "testpassword"
	)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupContext)
	params := s.Param().(openSettingsParam)

	cr, err := login.SetupUserWithLocalPassword(ctx,
		password,
		// Use a local password so that we don't need Gaia.
		chrome.FakeLogin(chrome.Creds{User: username, Pass: ""}),
		chrome.ExtraArgs("--disable-first-run-ui"),
	)
	if err != nil {
		s.Fatal("Failed to setup user: ", err)
	}
	defer cr.Close(cleanupContext)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Getting test API connection failed: ", err)
	}

	// Open OS Settings > lock screen.
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osPrivacy/lockScreen", func(context.Context) error { return nil })
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupContext)
	defer faillog.DumpUITreeOnError(cleanupContext, s.OutDir(), s.HasError, tconn)

	var expectedPath string
	if params.usePassword {
		// The page is password protected, confirm that we can access it with a password.
		if err := ossettings.ConfirmPassword(ctx, cr, password); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}
		expectedPath = "/osPrivacy/lockScreen"
	} else {
		// The page is password protected, cancelling should kick us out.
		if err := cancelPassword(ctx, cr); err != nil {
			s.Fatal("Failed to cancel: ", err)
		}
		expectedPath = "/osPrivacy"
	}

	// Check that the settings landed on the correct path.
	settingsPath, err := getSettingsPath(ctx, settings, cr)
	if err != nil {
		s.Fatal("Failed to get the settings path: ", err)
	}
	if settingsPath != expectedPath {
		s.Fatalf("Did not land on the correct settings path, expected %q, got %q", expectedPath, settingsPath)
	}
}

// cancelPassword enters the provided password in OS Settings, to open password-protected pages.
func cancelPassword(ctx context.Context, cr *chrome.Chrome) error {
	passwordNode := nodewith.Name("Confirm your password").Role(role.Dialog)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	uia := uiauto.New(tconn)
	if err := uia.WaitUntilExists(passwordNode.First())(ctx); err != nil {
		return errors.Wrap(err, "failed to find password dialog")
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	if err := keyboard.Type(ctx, "\x1b"); err != nil {
		return errors.Wrap(err, "failed to hit ESC")
	}

	if err := uia.WaitUntilGone(passwordNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait until password dialog is gone")
	}

	return nil
}

// getSettingsPath returns the current path of the OS Settings screen
func getSettingsPath(ctx context.Context, settings *ossettings.OSSettings, cr *chrome.Chrome) (string, error) {
	var pathname string
	err := settings.EvalJSWithShadowPiercer(ctx, cr, "window.location.pathname", &pathname)
	return pathname, err
}
