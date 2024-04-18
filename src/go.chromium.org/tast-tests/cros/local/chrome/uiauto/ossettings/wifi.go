// Copyright 2023 The ChromiumOS Authors
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

// LaunchAtWiFi launch OS settings and navigate to WiFi settings page.
func LaunchAtWiFi(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	condition := uiauto.New(tconn).Exists(nodewith.Name("Wi-Fi subpage back button"))
	return LaunchAtPageURL(ctx, tconn, cr, "networks?type=WiFi", condition)
}

// OpenJoinWiFiDialog opens the the "Join WiFi Network" dialog.
// This will attempt to enable Wi-Fi if it is disabled.
func OpenJoinWiFiDialog(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (_ *OSSettings, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	settings, err := LaunchAtPage(ctx, tconn, Internet)
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Network page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			settings.Close(ctx)
			settings = nil
		}
	}(cleanupCtx)

	return settings, uiauto.Combine("open Join WiFi dialog",
		// The Wi-Fi has to be enabled to launch the dialog.
		settings.SetToggleOption(cr, "Wi-Fi enable", true),
		settings.LeftClick(nodewith.Name("Add network connection").Role(role.Button)),
		settings.LeftClick(nodewith.NameContaining("Add Wi-Fi…").Role(role.Button)),
		settings.WaitUntilExists(nodewith.NameContaining("Join Wi-Fi network").Role(role.Dialog)),
	)(ctx)
}
