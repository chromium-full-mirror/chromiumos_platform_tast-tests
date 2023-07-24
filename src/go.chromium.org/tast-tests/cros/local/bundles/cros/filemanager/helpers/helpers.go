// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package helpers contains filemanager helpers.
package helpers

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast/core/errors"
)

// InstallRequiredExtensions installs both Google Docs Offline and Application Launcher for Drive extensions.
func InstallRequiredExtensions(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn) error {
	// TODO(b/193595364): Figure out why these extensions aren't being installed by default in tast tests.
	docsOfflineName := "Google Docs Offline"
	docsOfflineURL := "https://chrome.google.com/webstore/detail/google-docs-offline/ghbmnnjooekpmoecnnnilnnbdlolhkhi"
	docsOfflineExt := cws.App{Name: docsOfflineName, URL: docsOfflineURL}
	if err := cws.InstallApp(ctx, br, tconn, docsOfflineExt); err != nil {
		return errors.Wrap(err, "failed to install Google Docs Offline extension")
	}

	proxyExtName := "Application Launcher For Drive (by Google)"
	proxyExtURL := "https://chrome.google.com/webstore/detail/application-launcher-for/lmjegmlicamnimmfhcmpkclmigmmcbeh"
	proxyExt := cws.App{Name: proxyExtName, URL: proxyExtURL}
	if err := cws.InstallApp(ctx, br, tconn, proxyExt); err != nil {
		return errors.Wrap(err, "failed to install Application Launcher for Drive extension")
	}
	return nil
}
