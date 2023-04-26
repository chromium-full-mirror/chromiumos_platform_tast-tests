// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package instanttether

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TetherURL is the sub-URL for navigating to the Instant Tether page in OS Settings.
const TetherURL = "networks?type=Tether"

// NetworkAvailable checks if the network is available by navigating to a simple website.
func NetworkAvailable(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	c, err := cr.NewConn(ctx, "https://www.chromium.org/")
	if err != nil {
		return errors.Wrap(err, "failed to open browser")
	}
	defer c.Close()
	defer c.CloseTarget(ctx)
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name("The Chromium Projects").Role(role.Heading))(ctx); err != nil {
		return errors.Wrap(err, "page did not load")
	}
	return nil
}

// ConnectUsingNotification connects to the phone's mobile data by accepting the Instant Tether notification.
func ConnectUsingNotification(ctx context.Context, tconn *chrome.TestConn) error {
	if _, err := ash.WaitForNotification(ctx, tconn, 30*time.Second,
		ash.WaitTitleContains("Wi-Fi available via phone"),
		ash.WaitMessageContains("Data connection available from your"),
	); err != nil {
		return errors.Wrap(err, "failed to wait for Instant Tether notification")
	}

	ui := uiauto.New(tconn)
	btn := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)connect")).Ancestor(nodewith.Role(role.AlertDialog))
	if err := ui.LeftClick(btn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click Instant Tether notification's connect button")
	}

	return nil
}

// HandleFirstUseDialog accepts the first-use dialog for Instant Tether if it appears.
func HandleFirstUseDialog(ctx context.Context, cr *chrome.Chrome, sconn *ossettings.OSSettings) error {
	testing.ContextLog(ctx, "Waiting to see if first-use dialog is shown")
	firstUseText := nodewith.Role(role.StaticText).NameRegex(regexp.MustCompile("(?i)connect to new hotspot?"))
	if err := sconn.WithTimeout(5 * time.Second).WaitUntilExists(firstUseText)(ctx); err != nil {
		// If the first-use dialog doesn't appear, we don't need to do anything here.
		testing.ContextLog(ctx, "First-use dialog did not appear")
		return nil
	}

	connectBtn := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)connect")).Ancestor(nodewith.Role(role.Dialog))
	if err := sconn.LeftClick(connectBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click first-use dialog's connect button")
	}

	return nil
}
