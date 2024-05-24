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
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// Setup create tconn, browser, server etc.
func Setup(cleanupCtx context.Context, s *testing.State) (context.Context, *chrome.TestConn, *chrome.Chrome, *browser.Browser, string, func()) {
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)

	// For chrome
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// For ash chrome.TestConn
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// For browser.
	browserType := s.FixtValue().(fixture.FixtData).BrowserType()
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}

	// Lacros browser won't work with ash TestConn tconn, instead we need the TestConn from the browser.
	brTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get TestAPIConn from the browser: ", err)
	}
	// Close all existings tabs just in case some tabs are left from previous tests.
	if err := browser.CloseAllTabs(ctx, brTconn); err != nil {
		s.Fatal("Failed to close all tabs: ", err)
	}

	// Setup server to serve the files.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))

	// Add permission.
	br.GrantPermissions(ctx, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	// Return a cleanUp for the main test to call.
	cleanUpFunc := func() {
		browser.CloseAllTabs(ctx, brTconn)

		cancel()
		closeBrowser(cleanupCtx)
		conn.Close()
		conn.CloseTarget(cleanupCtx)
		srv.Close()
		faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")
	}

	return ctx, tconn, cr, br, srv.URL + "/", cleanUpFunc
}
