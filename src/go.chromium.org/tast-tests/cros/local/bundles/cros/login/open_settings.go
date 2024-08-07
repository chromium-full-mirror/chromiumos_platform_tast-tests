// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/login"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// configuredAuthType represents the configured authentication factor(s) during OOBE.
type configuredAuthType int

const (
	setupWithPassword configuredAuthType = iota
	setupWithPasswordAndPin
)

// settingsAuthType represents the used authentication factor to enter the authentication requested page.
type settingsAuthType int

const (
	authWithPassword settingsAuthType = iota
	authWithPin
	authCancel // this option is closing the authentication widget.
)

type openSettingsParam struct {
	configuredAuth configuredAuthType
	settingsAuth   settingsAuthType
	useAuthPanel   bool
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
				configuredAuth: setupWithPassword,
				settingsAuth:   authWithPassword,
				useAuthPanel:   false,
			},
		}, {
			Name: "auth_panel_with_password",
			Val: openSettingsParam{
				configuredAuth: setupWithPassword,
				settingsAuth:   authWithPassword,
				useAuthPanel:   true,
			},
		}, {
			Name: "auth_panel_with_pin",
			Val: openSettingsParam{
				configuredAuth: setupWithPasswordAndPin,
				settingsAuth:   authWithPin,
				useAuthPanel:   true,
			},
		}, {
			Name: "cancel",
			Val: openSettingsParam{
				configuredAuth: setupWithPassword,
				settingsAuth:   authCancel,
				useAuthPanel:   false,
			},
		}, {
			Name: "auth_panel_cancel",
			Val: openSettingsParam{
				configuredAuth: setupWithPassword,
				settingsAuth:   authCancel,
				useAuthPanel:   true,
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
	params := s.Param().(openSettingsParam)

	cr, err := setupUser(ctx, params, username, password, pin)
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
	switch params.settingsAuth {
	case authWithPassword:
		// The page is authentication protected, confirm that we can access it with a password.
		if err := ossettings.ConfirmPassword(ctx, cr, password); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}
		expectedPath = "/osPrivacy/lockScreen"
	case authWithPin:
		// The page is authentication protected, confirm that we can access it with a PIN.
		if err := ossettings.ConfirmPin(ctx, cr, pin); err != nil {
			s.Fatal("Failed to confirm pin: ", err)
		}
		expectedPath = "/osPrivacy/lockScreen"
	case authCancel:
		// The page is authentication protected, cancelling should kick us out.
		if err := ossettings.CancelPassword(ctx, cr); err != nil {
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

// setupUser configures a new chrome with the provided user credentials
func setupUser(ctx context.Context, params openSettingsParam, username, password, pin string) (*chrome.Chrome, error) {
	var authPanelState chrome.Option

	if params.useAuthPanel {
		authPanelState = chrome.EnableFeatures("UseAuthPanelInSession")
	} else {
		authPanelState = chrome.DisableFeatures("UseAuthPanelInSession")
	}
	loginOption := chrome.FakeLogin(chrome.Creds{User: username, Pass: ""})
	chromeArgs := chrome.ExtraArgs("--disable-first-run-ui")

	switch params.configuredAuth {
	case setupWithPassword:
		return login.SetupUserWithLocalPassword(ctx, password,
			authPanelState,
			loginOption,
			chromeArgs)
	case setupWithPasswordAndPin:
		return login.SetupUserWithLocalPasswordAndPin(ctx, password, pin,
			authPanelState,
			loginOption,
			chromeArgs)
	}
	return nil, errors.New("invalid setup type")
}

// getSettingsPath returns the current path of the OS Settings screen
func getSettingsPath(ctx context.Context, settings *ossettings.OSSettings, cr *chrome.Chrome) (string, error) {
	var pathname string
	err := settings.EvalJSWithShadowPiercer(ctx, cr, "window.location.pathname", &pathname)
	return pathname, err
}
