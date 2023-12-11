// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googledocs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// jamboardName represents the name of the Google Jamboard web area.
const jamboardName = "Google Jamboard"

var jamboardWebArea = nodewith.NameContaining(jamboardName).Role(role.RootWebArea)

// DeleteJamboard returns an action to delete the Jamboard.
func DeleteJamboard(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	moreActionsButton := nodewith.Name("More Actions").Role(role.PopUpButton).Ancestor(jamboardWebArea)
	removeItem := nodewith.Name("Remove").Role(role.MenuItem).Ancestor(jamboardWebArea)
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(jamboardWebArea)
	goToHomeButton := nodewith.Name("Go to Jamboard home screen").Role(role.Button).Ancestor(jamboardWebArea)

	return uiauto.NamedCombine("delete Jamboard",
		ui.WaitUntilExists(moreActionsButton),
		ui.DoDefault(moreActionsButton),
		ui.DoDefault(removeItem),
		ui.DoDefault(okButton),
		ui.DoDefault(goToHomeButton),
	)
}

// DeleteJamboardWithURL returns an action to open the Jamboard url and delete the Jamboard.
func DeleteJamboardWithURL(tconn *chrome.TestConn, cr *chrome.Chrome, url string) action.Action {
	return func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", url)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)

		if err := webutil.WaitForQuiescence(ctx, conn, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for the page to load")
		}
		return DeleteJamboard(tconn)(ctx)
	}
}
