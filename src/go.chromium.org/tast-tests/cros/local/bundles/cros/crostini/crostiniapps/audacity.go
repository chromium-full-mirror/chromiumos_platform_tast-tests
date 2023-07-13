// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostiniapps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/errors"
)

// VerifyAudacityLaunchAndClose verifies audacity window and item on shelf.
// It also closes audacity.
// Audacity mush have been launched.
func VerifyAudacityLaunchAndClose(ctx context.Context,
	tconn *chrome.TestConn,
	keyboard *input.KeyboardEventWriter,
	d screenshot.Differ) error {
	ud := uidetection.NewDefault(tconn)
	ui := uiauto.New(tconn)

	// To avoid matching against the splash screen with title "Audacity is
	// starting up...", also look for the File menu in the window.
	audacityWindow := nodewith.Name("Audacity").Role(role.Window).First()
	if err := uiauto.Combine("wait for Audacity",
		ui.WithTimeout(2*time.Minute).WaitUntilExists(audacityWindow),
		ud.WaitUntilExists(uidetection.Word("File").WithinA11yNode(audacityWindow)),
		ui.MouseClickAtLocation(0, coords.Point{X: 0, Y: 0}),
		d.DiffWindow(ctx, "audacity", screenshot.Retries(5), screenshot.RetryInterval(5*time.Second)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find Audacity window")
	}

	// Check audacity is displayed on shelf.
	shelfItems, err := ash.ShelfItems(ctx, tconn)
	found := false
	if err != nil {
		return errors.Wrap(err, "failed to get shelf items")
	}
	for _, shelfItem := range shelfItems {
		if shelfItem.Title == "Audacity" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("failed to find Audacity on shelf")
	}

	return uiauto.Combine("quit Audacity",
		keyboard.AccelAction("ctrl+Q"),
		ui.WaitUntilGone(audacityWindow),
	)(ctx)
}
