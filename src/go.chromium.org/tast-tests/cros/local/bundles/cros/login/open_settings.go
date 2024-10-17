// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/auth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

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
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPassword,
				InSessionAuth:  auth.AuthWithPassword,
				UseAuthPanel:   false,
			},
		}, {
			Name: "auth_panel_with_password",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPassword,
				InSessionAuth:  auth.AuthWithPassword,
				UseAuthPanel:   true,
			},
		}, {
			Name: "auth_panel_with_pin",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPasswordAndPin,
				InSessionAuth:  auth.AuthWithPin,
				UseAuthPanel:   true,
			},
		}, {
			Name: "cancel",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPassword,
				InSessionAuth:  auth.AuthCancel,
				UseAuthPanel:   false,
			},
		}, {
			Name: "auth_panel_cancel",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPassword,
				InSessionAuth:  auth.AuthCancel,
				UseAuthPanel:   true,
			},
		}},
	})
}

func OpenSettings(ctx context.Context, s *testing.State) {
	const (
		username = "testuser@gmail.com"
		password = "testpassword"
		pin      = "15050410"
	)

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer userutil.ResetUsers(cleanupContext)
	params := s.Param().(auth.InSessionParam)

	cr, err := auth.SetupUser(ctx, params, username, password, pin)
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
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupContext, s.OutDir(), s.HasError, cr, "ui_dump")

	var expectedPath string
	switch params.InSessionAuth {
	case auth.AuthWithPassword:
		// The page is authentication protected, confirm that we can access it with a password.
		if err := auth.ConfirmPassword(ctx, cr, password); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}
		expectedPath = "/osPrivacy/lockScreen"
	case auth.AuthWithPin:
		// The page is authentication protected, confirm that we can access it with a PIN.
		if err := auth.ConfirmPin(ctx, cr, pin); err != nil {
			s.Fatal("Failed to confirm pin: ", err)
		}
		expectedPath = "/osPrivacy/lockScreen"
	case auth.AuthCancel:
		// The page is authentication protected, cancelling should kick us out.
		if err := auth.CancelPassword(ctx, cr); err != nil {
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

// getSettingsPath returns the current path of the OS Settings screen
func getSettingsPath(ctx context.Context, settings *ossettings.OSSettings, cr *chrome.Chrome) (string, error) {
	var pathname string
	err := settings.EvalJSWithShadowPiercer(ctx, cr, "window.location.pathname", &pathname)
	return pathname, err
}
