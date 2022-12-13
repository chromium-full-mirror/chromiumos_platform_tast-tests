// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto/cws"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/testing"
)

// EnsureDocsOfflineInstalled ensures that docs offline extension is installed
// for the browser. `br` is where the extension is installed to. `tconn` is a
// TestConn to ash-chrome.
func EnsureDocsOfflineInstalled(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn) error {
	const (
		docsOfflineID   = "ghbmnnjooekpmoecnnnilnnbdlolhkhi"
		docsOfflineName = "Google Docs Offline"
		docsOfflineURL  = "https://chrome.google.com/webstore/detail/google-docs-offline/ghbmnnjooekpmoecnnnilnnbdlolhkhi"
	)
	docsOfflineExt := cws.App{Name: docsOfflineName, URL: docsOfflineURL}

	isInstalled, err := ash.ChromeAppInstalled(ctx, tconn, docsOfflineID)
	if err != nil {
		return errors.Wrap(err, "failed to check existence of docs offline extension")
	}

	if isInstalled {
		testing.ContextLog(ctx, "Docs offline extension is already installed")
		return nil
	}

	testing.ContextLog(ctx, "Install docs offline extension")
	return cws.InstallApp(ctx, br, tconn, docsOfflineExt)
}

// EnsureDocsOfflineEnabled ensures that docs offline extension is installed for
// the browser and the current active user has it enabled in Drive's settings.
// This function should be called before opening any docs if offline capability
// is desired.
func EnsureDocsOfflineEnabled(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn) error {
	if err := EnsureDocsOfflineInstalled(ctx, br, tconn); err != nil {
		return errors.Wrap(err, "failed to install Docs offline extension")
	}

	// Open Drive settings page.
	conn, err := br.NewConn(ctx, "https://drive.google.com/settings")
	if err != nil {
		return errors.Wrap(err, "failed to open Drive settings")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	// Wait for settings page to load and sync the account settings.
	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for Drive settings to load")
	}

	// Make sure the "offline" checkbox is checked.
	testing.ContextLog(ctx, "Making sure offline support is enabled")
	if err := conn.Call(ctx, nil, `() => {
		let offlineCheckbox = document.getElementsByName('offline')[0];
		if (!offlineCheckbox.checked)
			offlineCheckbox.click();
	}`); err != nil {
		return errors.Wrap(err, "failed to ensure offline checkbox checked")
	}

	testing.ContextLog(ctx, "Docs offline support enabled")
	return nil
}
