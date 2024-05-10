// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakepwa contains the library of fake VC PWA app.
package fakepwa

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/testing"
)

// VcPwaUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcPwaUI struct {
	common.VcWebApp
	tconn  *chrome.TestConn
	window *ash.Window
}

// SetupServerAndPermission installs the app, sets its permission and returns its appID.
func SetupServerAndPermission(ctx context.Context, br *browser.Browser, s *testing.State, tconn *chrome.TestConn) string {
	// Grant permission.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	br.GrantPermissions(ctx, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	vcPwaFullURL := srv.URL + common.VcAppURL
	if err := apps.InstallPWAForURL(ctx, tconn, br, vcPwaFullURL, 15*time.Second); err != nil {
		s.Fatal("Failed to install PWA for URL: ", err)
	}

	appID, err := apps.InstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool {
		return app.Name == common.VcAppName
	}, &testing.PollOptions{Timeout: 5 * time.Second})
	if err != nil {
		s.Fatal("Failed to get appID: ", err)
	}

	// Wait for the app to show.
	window, err := ash.WaitForAppWindow(ctx, tconn, appID)
	if err != nil {
		s.Fatal("Failed to wait for app window: ", err)
	}

	if err := window.CloseWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to close app: ", err)
	}

	return appID
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

// CloseApp closes the app with appID inside VcPwaUI.
func (pwaUI *VcPwaUI) CloseApp(ctx context.Context) error {
	return pwaUI.window.CloseWindow(ctx, pwaUI.tconn)
}
