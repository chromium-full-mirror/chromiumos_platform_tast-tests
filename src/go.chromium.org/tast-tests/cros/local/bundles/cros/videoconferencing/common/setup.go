// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common this file contains shared logic between extension, pwa and tab.
package common

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// Setup create tconn, browser, server etc.
func Setup(cleanupCtx context.Context, s *testing.State) (context.Context, *chrome.TestConn, *chrome.Chrome, string, func()) {
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	conn, err := cr.NewConn(ctx, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}

	// Close all existings tabs just in case some tabs are left from previous tests.
	if err := browser.CloseAllTabs(ctx, tconn); err != nil {
		s.Fatal("Failed to close all tabs: ", err)
	}

	// Setup server to serve the files.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))

	// Add permission.
	browser.GrantPermissions(ctx, tconn, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	// Return a cleanUp for the main test to call.
	cleanUpFunc := func() {
		browser.CloseAllTabs(ctx, tconn)

		cancel()
		conn.Close()
		conn.CloseTarget(cleanupCtx)
		srv.Close()
		faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")
	}

	return ctx, tconn, cr, srv.URL + "/", cleanUpFunc
}
