// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakevctab contains the library of fake VC tab.
package fakevctab

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
)

// VcTabUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcTabUI struct {
	common.VcWebApp
	tconn  *chrome.TestConn
	window *ash.Window
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

// Close closes the tab with id inside VcTabUI.
func (tabUI *VcTabUI) Close(ctx context.Context) error {
	return tabUI.window.CloseWindow(ctx, tabUI.tconn)
}
