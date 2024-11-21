// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/identitycuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/auth"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: OpenPasswordManager,
		Desc: "Open Password manager and access them with a password or pin",
		Contacts: []string{
			"cros-lurs@google.com",
			"iscsi@google.com",
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
			},
		}, {
			Name: "with_pin",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPasswordAndPin,
				InSessionAuth:  auth.AuthWithPin,
			},
		}, {
			Name: "with_pin_only",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPin,
				InSessionAuth:  auth.AuthWithPin,
			},
		}, {
			Name: "cancel",
			Val: auth.InSessionParam{
				ConfiguredAuth: auth.SetupWithPassword,
				InSessionAuth:  auth.AuthCancel,
			},
		}},
	})
}

func OpenPasswordManager(ctx context.Context, s *testing.State) {
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
	defer faillog.DumpUITreeOnError(cleanupContext, s.OutDir(), s.HasError, tconn)

	// Populate password manager database.
	// Open the Password manager in a new browser tab.
	_, err = identitycuj.OpenNewURL(ctx, cr, "chrome://password-manager/passwords")
	if err != nil {
		s.Fatal("Failed to open Password Manager: ", err)
	}

	// Wait for webpage to load its content after we accepted the warning.
	if err := tconn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Failed to wait document is ready: ", err)
	}

	// The chrome.passwordsPrivate.addPassword only works if the password
	// manager page is open.
	if err := tconn.Eval(ctx, `tast.promisify(chrome.passwordsPrivate.addPassword({
    url: "example",
    username: "testuser",
    password: "testpassword",
    note: "This is a test password",
    useAccountStore: true
  }));`, nil); err != nil {
		s.Fatal("Failed to execute JS expression: ", err)
	}

	// Close the only opened tab.
	tab, err := getTheOnlyOpenedTab(ctx, tconn)

	if err != nil {
		s.Fatal("Failed to get the only opened tab")
	}

	err = browser.CloseTabsByID(ctx, tconn, []int{tab.ID})

	if err != nil {
		s.Fatal("Failed to close the password manager tab: ", err)
	}

	// Try to open the previously stored example account password manager subpage.
	_, err = identitycuj.OpenNewURL(ctx, cr, "chrome://password-manager/passwords/example")
	if err != nil {
		s.Fatal("Failed to open Password Manager: ", err)
	}

	// Check the requested authentication protected page is on the current tab.
	tab, err = getTheOnlyOpenedTab(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the password manager tab")
	}
	if tab.URL != "chrome://password-manager/passwords/example" {
		s.Fatal("Failed to open the requested webpage: ", err)
	}

	switch params.InSessionAuth {
	case auth.AuthWithPassword:
		// The page is authentication protected, confirm that we can access it with a password.
		if err := auth.ConfirmPassword(ctx, cr, password); err != nil {
			s.Fatal("Failed to confirm password: ", err)
		}

		// The ConfirmPassword is checking the auth dialog disappeared.
		// This if checks that we are still on the requested URL it means we can access to the
		// user credentials.
		tab, err := getTheOnlyOpenedTab(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to find the password manager tab")
		}
		if tab.URL != "chrome://password-manager/passwords/example" {
			s.Fatal("Failed to access with password to the protected page: ", err)
		}
	case auth.AuthWithPin:
		// The page is authentication protected, confirm that we can access it with a PIN.
		if err := auth.ConfirmPin(ctx, cr, pin, params.ConfiguredAuth == auth.SetupWithPin); err != nil {
			s.Fatal("Failed to confirm pin: ", err)
		}

		// The ConfirmPin is checking the auth dialog disappeared.
		// This if checks that we are still on the requested URL it means we can access to the
		// user credentials.
		tab, err := getTheOnlyOpenedTab(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to find the password manager tab")
		}
		if tab.URL != "chrome://password-manager/passwords/example" {
			s.Fatal("Failed to access with PIN to the protected page: ", err)
		}
	case auth.AuthCancel:
		// The page is authentication protected, cancelling should kick us out.
		if err := auth.CancelPassword(ctx, cr); err != nil {
			s.Fatal("Failed to cancel: ", err)
		}

		// The CancelPassword is checking the auth dialog disappeared.
		// This if checks that without a successful authentication the browser navigates back
		// to the main password manager page.
		tab, err := getTheOnlyOpenedTab(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to find the password manager tab")
		}
		if tab.URL != "chrome://password-manager/passwords" {
			s.Fatal("Failed to navigate to the password manager page after cancel: ", err)
		}
	}
}

// getTheOnlyOpenedTab returns the opened tab that currently is in the browser window.
// The browser is given via |tconn|.
func getTheOnlyOpenedTab(ctx context.Context, tconn *chrome.TestConn) (*browser.Tab, error) {
	tabs, err := browser.CurrentTabs(ctx, tconn)

	if err != nil {
		return nil, err
	}

	if len(tabs) == 0 {
		return nil, errors.New("the browser has no open tabs")
	}

	if len(tabs) > 1 {
		return nil, errors.New("multiple tabs are open in the browser")
	}

	return &tabs[0], nil
}
