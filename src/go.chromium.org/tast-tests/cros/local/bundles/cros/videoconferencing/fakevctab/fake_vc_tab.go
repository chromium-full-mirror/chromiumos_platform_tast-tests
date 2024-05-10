// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakevctab contains the library of fake VC tab.
package fakevctab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/testing"
)

// VcTabUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcTabUI struct {
	common.VcWebApp
	tconn  *chrome.TestConn
	window *ash.Window
}

// SetupServerAndPermission sets the permission for the tab and returns the url.
func SetupServerAndPermission(ctx context.Context, br *browser.Browser, s *testing.State) string {
	// Grant permission.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	br.GrantPermissions(ctx, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	return srv.URL + common.VcAppURL
}

// LaunchTab opens a new tab for the url.
func LaunchTab(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, url string) (*VcTabUI, error) {
	if _, err := br.NewTab(ctx, url); err != nil {
		return nil, err
	}

	window, err := ash.WaitForAnyWindow(ctx, tconn, func(w *ash.Window) bool {
		return strings.Contains(w.Title, common.VcAppName) && w.IsVisible && !w.IsAnimating
	})

	if err != nil {
		return nil, err
	}

	tabUI := VcTabUI{common.VcWebApp{UI: uiauto.New(tconn)}, tconn, window}

	if err := tabUI.WaitUntilAllButtonsExists(ctx); err != nil {
		return nil, err
	}

	return &tabUI, nil
}

// CloseTab closes the tab with id inside VcTabUI.
func (tabUI *VcTabUI) CloseTab(ctx context.Context) error {
	return tabUI.window.CloseWindow(ctx, tabUI.tconn)
}
