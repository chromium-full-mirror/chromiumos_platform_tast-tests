// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskswitchcuj

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
)

// simpleWebsites are websites to be opened in individual browsers
// with no additional setup required.
// 1. Chromium issue tracker -- considerable amount of elements.
// 2. About Version -- lightweight website.
var simpleWebsites = []string{
	"https://bugs.chromium.org/p/chromium/issues/list",
	chrome.VersionURL,
}

// openChromeTabs opens Chrome tabs and returns the number of windows
// that were opened.
//
// This function opens an individual window for each URL in
// simpleWebsites. It also opens a window with multiple tabs, to
// increase RAM pressure during the test.
func openChromeTabs(ctx context.Context, tconn, bTconn *chrome.TestConn, br *browser.Browser, bt browser.Type, tabletMode bool) (int, error) {
	const numExtraWebsites = 2

	// Keep track of the initial number of windows, to ensure
	// we open the right number of windows.
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get window list")
	}
	initialNumWindows := len(ws)

	// Install Meet PWA.
	if err := apps.InstallPWAForURL(ctx, tconn, br, "https://meet.google.com", 30*time.Second); err != nil {
		return 0, errors.Wrap(err, "failed to install Meet PWA")
	}

	// Open up a single window with a couple of tabs, to increase RAM pressure.
	tabs, err := cuj.NewTabs(ctx, br, false, numExtraWebsites)
	if err != nil {
		return 0, errors.Wrap(err, "failed to bulk open tabs")
	}

	// Lacros specific setup to close "New Tab" window.
	if bt == browser.TypeLacros {
		// Don't include the "New Tab" window in the initial window count.
		initialNumWindows--

		if err := browser.CloseTabByTitle(ctx, bTconn, "New Tab"); err != nil {
			return 0, errors.Wrap(err, `failed to close "New Tab" tab`)
		}
	}

	// Open up individual window for each website in simpleWebsites.
	taskSwitchTabs, err := cuj.NewTabsByURLs(ctx, br, true, simpleWebsites)
	if err != nil {
		return 0, err
	}
	tabs = append(tabs, taskSwitchTabs...)

	// Close all current connections to tabs, because we don't need them.
	for _, t := range tabs {
		if err := t.Conn.Close(); err != nil {
			return 0, errors.Wrapf(err, "failed to close connection to %s", t.URL)
		}
	}

	if !tabletMode {
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateNormal)
		}); err != nil {
			return 0, errors.Wrap(err, "failed to set each window to normal state")
		}
	}

	// Expected number of browser windows should include the number of
	// websites in |simpleWebsites| and the window with multiple tabs.
	// This count purposefully does not include the PWA, since the PWA is
	// technically treated as its own app, since it has its own icon in
	// the shelf.
	expectedNumBrowserWindows := len(simpleWebsites) + 1
	if ws, err := ash.GetAllWindows(ctx, tconn); err != nil {
		return 0, errors.Wrap(err, "failed to get window list after opening Chrome tabs")
	} else if expectedNumWindows := expectedNumBrowserWindows + initialNumWindows + 1; len(ws) != expectedNumWindows {
		return 0, errors.Wrapf(err, "unexpected number of windows open after launching Chrome tabs, got: %d, expected: %d", len(ws), expectedNumWindows)
	}

	return expectedNumBrowserWindows, nil
}
