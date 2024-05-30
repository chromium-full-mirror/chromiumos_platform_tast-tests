// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// DisconnectVPN disconnects from a VPN network and ensure it's disconnected.
// This utility can only work when the detail page of the desired VPN network is opened.
func DisconnectVPN(ctx context.Context, tconn *chrome.TestConn) error {
	settings := New(tconn)
	return uiauto.Combine("disconnect VPN",
		settings.LeftClick(nodewith.Name("Disconnect").Role(role.Button)),
		settings.WaitUntilExists(nodewith.Name("Not Connected").Role(role.StaticText)),
	)(ctx)
}

// ForgetVPN forgets a VPN network and ensure it's forgotten.
// This utility can only work when the detail page of the desired VPN network is opened.
func ForgetVPN(ctx context.Context, tconn *chrome.TestConn, vpnName string) error {
	settings := New(tconn)
	return uiauto.Combine("forget VPN",
		settings.LeftClick(nodewith.Name("Forget").Role(role.Button)),
		settings.WaitUntilExists(nodewith.Name("VPN").Role(role.Heading)),
		settings.WaitUntilGone(nodewith.NameContaining(vpnName)),
	)(ctx)
}

// OpenJoinVPNDialog launches the "Join VPN network" dialog from OS-Settings.
func OpenJoinVPNDialog(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (_ *OSSettings, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	settings, err := Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the OS settings page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			settings.Close(ctx)
		}
	}(cleanupCtx)

	if err := settings.NavigateToPageURL(ctx, cr, "internet", settings.Exists(Internet)); err != nil {
		return nil, errors.Wrap(err, "failed to open the OS settings page")
	}

	if err := uiauto.Combine(`open the "Join VPN network" dialog`,
		settings.LeftClick(AddConnectionButton),
		settings.LeftClick(nodewith.NameContaining("Add built-in VPN").Role(role.Button)),
	)(ctx); err != nil {
		return nil, err
	}

	return settings, nil
}
