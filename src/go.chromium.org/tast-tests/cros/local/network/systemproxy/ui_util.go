// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package systemproxy contains utility functions to authenticate to the system-proxy daemon.
package systemproxy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DoSystemProxyAuthentication authenticates to system-proxy with `username` and `password` by clicking on the system-proxy notification
// which informs the user that system-proxy requires credentials and entering the proxy credentials in the system-proxy dialog.
// If system-proxy is not asking for credentials or in case of failure, returns an error.
func DoSystemProxyAuthentication(ctx context.Context, tconn *chrome.TestConn, username, password string) error {
	const (
		notificationTitle = "Sign in"
		uiTimeout         = 10 * time.Second
	)

	// TODO(b/290289663): Remove this check once qsRevamp is launched.
	qsRevampEnabled, err := quicksettings.QsRevampEnabled(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get QsRevamp state")
	}

	if qsRevampEnabled {
		if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to show the notification center")
		}
	} else {
		if err := quicksettings.Show(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to show system tray")
		}
	}

	if _, err := ash.WaitForNotification(ctx, tconn, uiTimeout, ash.WaitTitle(notificationTitle)); err != nil {
		return errors.Wrapf(err, "failed waiting %v for system-proxy notification", uiTimeout)
	}

	ui := uiauto.New(tconn)
	if err := ui.WithPollOpts(testing.PollOptions{Interval: 2 * time.Second, Timeout: uiTimeout}).LeftClick(nodewith.Name(notificationTitle).Role(role.StaticText))(ctx); err != nil {
		return errors.Wrap(err, "failed finding notification and clicking it")
	}

	// Introduce Credentials in the system-proxy dialog.
	dialog := nodewith.HasClass("RequestSystemProxyCredentialsView").First()
	if err := ui.WithPollOpts(testing.PollOptions{Interval: 2 * time.Second, Timeout: uiTimeout}).WaitUntilExists(dialog)(ctx); err != nil {
		return errors.Wrap(err, "failed to find system-proxy dialog")
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Type(ctx, username); err != nil {
		return errors.Wrap(err, "failed to type username")
	}
	// Move focus to password text field.
	if err := kb.Accel(ctx, "Tab"); err != nil {
		return errors.Wrap(err, "failed to navigate via tab to the password text field")
	}
	if err := kb.Type(ctx, password); err != nil {
		return errors.Wrap(err, "failed to type password")
	}

	okButton := nodewith.Role(role.Button).Name("Sign in").Ancestor(dialog)
	if err := ui.WithTimeout(uiTimeout).LeftClick(okButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the sign in button")
	}
	return nil
}
