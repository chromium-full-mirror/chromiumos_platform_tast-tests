// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webutil

import (
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LoginGoogleAccount logs in Google account on a browser page.
// It assumes the browser has requested user login.
// It is used when testing on guest/incognito mode where device login user cannot be used for authentication.
func LoginGoogleAccount(ctx context.Context, cr *chrome.Chrome, account, password string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect Test API")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard")
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)

	accountField := nodewith.Name("Email or phone").Role(role.TextField).Editable()
	showPassword := nodewith.Name("Show password").Role(role.CheckBox).Focusable()
	passwordField := nodewith.Name("Enter your password").Role(role.TextField).Editable()

	confirmInput := func(finder *nodewith.Finder, input string) uiauto.Action {
		return func(ctx context.Context) error {
			return testing.Poll(ctx, func(ctx context.Context) error {
				if err := uiauto.Combine("type input",
					ui.LeftClickUntilFocused(finder),
					kb.AccelAction("Ctrl+A"),
					kb.TypeAction(input),
				)(ctx); err != nil {
					return err
				}
				node, err := ui.Info(ctx, finder)
				if err != nil {
					return err
				}
				if node.Value == input {
					return nil
				}
				return errors.Errorf("%s is incorrect: got: %v; want: %v", node.Name, node.Value, input)
			}, &testing.PollOptions{Timeout: 30 * time.Second})
		}
	}

	changeLanguage := func(ctx context.Context) error {
		heading := nodewith.Name("Sign in").Role(role.Heading)
		if err := ui.WaitUntilExists(heading)(ctx); err == nil {
			return nil
		}
		changeLanguage := nodewith.Role(role.ListBox).Collapsed().Vertical()
		englishOption := nodewith.NameContaining("English (United States)").Role(role.ListBoxOption)
		return uiauto.NamedAction("change the language", uiauto.Combine("change the language",
			ui.LeftClick(changeLanguage),
			ui.LeftClick(englishOption),
			// The page might take longer to change the display to "English (United States)" language in low-end DUTs.
			ui.WithTimeout(30*time.Second).WaitUntilExists(accountField),
		))(ctx)
	}

	warning := nodewith.Name("Protect your account").Role(role.StaticText)
	confirm := nodewith.Name("CONFIRM").Role(role.Button).Focusable()

	return uiauto.Combine("login to browser",
		// Although the browser has been changed to English, the login page will still display another default language in low-end DUTs.
		changeLanguage,
		confirmInput(accountField, account),
		kb.AccelAction("Enter"),
		ui.WaitUntilExists(passwordField),
		ui.DoDefault(showPassword),
		confirmInput(passwordField, password),
		kb.AccelAction("Enter"),
		uiauto.IfSuccessThen(
			ui.WaitUntilExists(warning),
			uiauto.NamedAction(`click the "CONFIRM" button`, ui.LeftClick(confirm)),
		),
	)(ctx)
}
