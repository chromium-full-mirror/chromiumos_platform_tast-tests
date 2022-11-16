// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto/cws"
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
