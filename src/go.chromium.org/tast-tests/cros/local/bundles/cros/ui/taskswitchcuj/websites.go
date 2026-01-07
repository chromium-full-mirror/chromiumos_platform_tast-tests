// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package taskswitchcuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// simpleWebsites are websites to be opened in individual browsers
// with no additional setup required.
// 1. Chromium issue tracker -- considerable amount of elements.
// 2. About Version -- lightweight website.
var simpleWebsites = []string{
	"https://issues.chromium.org/issues?q=status:open%20status:closed",
	"https://chromium.org/Home",
}

// openChromeTabs opens Chrome tabs and returns the number of windows
// that were opened.
//
// This function opens an individual window for each URL in
// simpleWebsites. It also opens a window with multiple tabs, to
// increase RAM pressure during the test.
func openChromeTabs(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, tabletMode bool) (int, error) {
	const numExtraWebsites = 2

	// Keep track of the initial number of windows, to ensure
	// we open the right number of windows.
	initialWindows, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get window list")
	}
	initialNumWindows := len(initialWindows)

	// Open up a single window with a couple of tabs, to increase RAM pressure.
	tabs, err := cuj.NewTabs(ctx, cr, false, numExtraWebsites)
	if err != nil {
		return 0, errors.Wrap(err, "failed to bulk open tabs")
	}

	// Open up individual window for each website in simpleWebsites.
	taskSwitchTabs, err := cuj.NewTabsByURLs(ctx, cr, true, simpleWebsites)
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
	expectedNumBrowserWindows := len(simpleWebsites) + 1
	if ws, err := ash.GetAllWindows(ctx, tconn); err != nil {
		return 0, errors.Wrap(err, "failed to get window list after opening Chrome tabs")
	} else if expectedNumWindows := expectedNumBrowserWindows + initialNumWindows; len(ws) != expectedNumWindows {
		// Print out two window list mismatch details.
		windowMismatchMsg := cuj.LogWindowMismatch(ctx, initialWindows, ws)
		return 0, errors.Wrapf(err, "unexpected number of windows open after launching Chrome tabs, got: %d, expected: %d; %s", len(ws), expectedNumWindows, windowMismatchMsg)
	}

	return expectedNumBrowserWindows, nil
}

func openPWA(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (func(ctx context.Context) error, error) {
	const (
		// This value is used to properly uninstall the app during test
		// cleanup. This name is different than apps.Meet.Name, which
		// is why it is set separately.
		nameInSettingsApp = "Google Meet"

		// pwaURL is the url used to install the PWA.
		pwaURL = "https://meet.google.com"

		installTimeout = 2 * time.Minute
	)

	appID := apps.Meet.ID

	alreadyInstalled, err := ash.ChromeAppInstalled(ctx, tconn, appID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check whether %s PWA has already been installed", nameInSettingsApp)
	}

	if alreadyInstalled {
		if err := apps.Launch(ctx, tconn, appID); err != nil {
			return nil, errors.Wrapf(err, "failed to launch %s PWA", nameInSettingsApp)
		}
	} else {
		var startTime = time.Now()
		if err := apps.InstallPWAForURL(ctx, cr, pwaURL, installTimeout); err != nil {
			return nil, errors.Wrapf(err, "failed to install and launch %s PWA", nameInSettingsApp)
		}
		duration := time.Since(startTime)
		testing.ContextLogf(ctx, "Install of %s PWA completed in %v", nameInSettingsApp, duration)
	}

	if err := prompts.ClearPotentialPrompts(tconn, 5*time.Second,
		prompts.NewMeetPrompt,
		prompts.ReceiveNotificationsPrompt,
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to dismiss Google Meet prompts")
	}

	return func(ctx context.Context) error {
		return ossettings.UninstallApp(ctx, tconn, cr, nameInSettingsApp, appID)
	}, nil
}
