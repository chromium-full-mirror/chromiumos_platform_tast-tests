// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakepwa contains the library of fake VC PWA app.
package fakepwa

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VcPwaUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcPwaUI struct {
	common.VcWebApp
	tconn  *chrome.TestConn
	window *ash.Window
}

// InstallPwa installs the pwa and returns its appID.
func InstallPwa(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, url string) (string, error) {
	if err := apps.InstallPWAForURL(ctx, tconn, br, url, 15*time.Second); err != nil {
		return "", errors.Wrap(err, "failed to InstallPWAForURL")
	}

	appID, err := apps.InstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool {
		return app.Name == common.VcAppName
	}, &testing.PollOptions{Timeout: 5 * time.Second})
	if err != nil {
		return "", errors.Wrap(err, "failed to InstalledAppID")
	}

	if err := ash.WaitForApp(ctx, tconn, appID, 15*time.Second); err != nil {
		return "", errors.Wrap(err, "failed to WaitForApp")
	}

	if err := apps.Close(ctx, tconn, appID); err != nil {
		return "", errors.Wrap(err, "failed to close app")
	}

	// Wait for the app to close.
	if err := ash.WaitForAppClosed(ctx, tconn, appID); err != nil {
		return "", errors.Wrap(err, "failed to WaitForAppClosed")
	}

	return appID, nil
}

// LaunchApp opens an app with appID.
func LaunchApp(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, appID string) (*VcPwaUI, error) {
	if err := apps.Launch(ctx, tconn, appID); err != nil {
		return nil, err
	}

	// Wait for the app to show.
	window, err := ash.WaitForAppWindow(ctx, tconn, appID)
	if err != nil {
		return nil, err
	}

	pwaUI := VcPwaUI{common.VcWebApp{UI: uiauto.New(tconn)}, tconn, window}

	if err := pwaUI.WaitUntilAllButtonsExists(ctx); err != nil {
		return nil, err
	}

	return &pwaUI, nil
}

// Close closes the app with appID inside VcPwaUI.
func (pwaUI *VcPwaUI) Close(ctx context.Context) error {
	return pwaUI.window.CloseWindow(ctx, pwaUI.tconn)
}
