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
	"go.chromium.org/tast/core/testing"
)

// InstalledExtension represents the minimal information required to identify an
// extension from chrome://extensions-internals.
type InstalledExtension struct {
	// ID is the extensions installed ID.
	ID string

	// Name is the extensions installed name.
	Name string

	// Version is the extensions installed version.
	Version string
}

// InstallRequiredExtensions installs both Google Docs Offline and Application Launcher for Drive extensions.
func InstallRequiredExtensions(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn) error {
	docsOfflineExtensionID := "ghbmnnjooekpmoecnnnilnnbdlolhkhi"
	applicationLauncherForDriveExtensionID := "lmjegmlicamnimmfhcmpkclmigmmcbeh"

	conn, err := br.NewTab(ctx, "chrome://extensions-internals")
	if err != nil {
		return errors.Wrap(err, "failed to open chrome://extensions-internals tab")
	}
	defer conn.Close()

	var extensions []InstalledExtension
	if err := conn.Eval(ctx, `JSON.parse(document.querySelector('pre').innerText)`, &extensions); err != nil {
		return errors.Wrap(err, "failed to extract installed extensions")
	}

	idsAlreadyInstalled := 0
	for _, ext := range extensions {
		if ext.ID == docsOfflineExtensionID || ext.ID == applicationLauncherForDriveExtensionID {
			testing.ContextLogf(ctx, "Extension %q (%s) v%s already installed", ext.Name, ext.ID, ext.Version)
			idsAlreadyInstalled++
		}
		if idsAlreadyInstalled == 2 {
			return nil
		}
	}

	// TODO(b/193595364): Figure out why these extensions aren't being installed by default in tast tests.
	docsOfflineName := "Google Docs Offline"
	docsOfflineURL := "https://chrome.google.com/webstore/detail/google-docs-offline/" + docsOfflineExtensionID
	docsOfflineExt := cws.App{Name: docsOfflineName, URL: docsOfflineURL}
	if err := cws.InstallApp(ctx, br, tconn, docsOfflineExt); err != nil {
		return errors.Wrap(err, "failed to install Google Docs Offline extension")
	}

	proxyExtName := "Application Launcher For Drive (by Google)"
	proxyExtURL := "https://chrome.google.com/webstore/detail/application-launcher-for/" + applicationLauncherForDriveExtensionID
	proxyExt := cws.App{Name: proxyExtName, URL: proxyExtURL}
	if err := cws.InstallApp(ctx, br, tconn, proxyExt); err != nil {
		return errors.Wrap(err, "failed to install Application Launcher for Drive extension")
	}
	return nil
}
