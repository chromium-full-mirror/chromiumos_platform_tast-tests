// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosinfo"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Activate,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests browser activation via shelf controller and via accelerator shortcuts",
		Contacts:     []string{"lacros-team@google.com", "neis@google.com"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Params: []testing.Param{{
			Name:    "no_keep_alive",
			Fixture: "lacros",
		}, {
			Name:    "keep_alive",
			Fixture: "lacrosKeepAlive",
		}},
	})
}

type appState string

const (
	appStateClosed     appState = "Closed"
	appStateBackground appState = "Background"
	appStateForeground appState = "Foreground"
	appStateOtherDesk  appState = "OtherDesk"
)

const urlForPreparedTab = "chrome://version/"

func Activate(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}

	// Create a new desk and switch to it. This is where we will later
	// perform the activation. The original desk is used only in case
	// appStateOtherDesk.
	if err := ash.CreateNewDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to create new desk: ", err)
	}
	if err := ash.ActivateDeskAtIndex(ctx, tconn, 1); err != nil {
		s.Fatal("Failed to activate new desk: ", err)
	}
	defer func() {
		info, err := ash.GetDesksInfo(ctx, tconn)
		if err != nil {
			s.Error("Failed to get desks info: ", err)
		}
		if info.NumDesks != 2 {
			s.Errorf("Unexpected desk count: got %d, want 2", info.NumDesks)
		}
		if info.ActiveDeskIndex != 1 {
			s.Errorf("Unexpected active desk index: got %d, want 1", info.ActiveDeskIndex)
		}
		if err := ash.RemoveActiveDesk(ctx, tconn); err != nil {
			s.Error("Failed to remove extra desk: ", err)
		}
	}()

	for _, param := range []struct {
		name                string
		browserPrecondition appState
		activateBrowser     func(ctx context.Context, tconn *chrome.TestConn) error
		expectPreparedTab   bool
		expectNewTabPage    bool
	}{
		// Ctrl-t
		{
			name:                "closed_ctrl-t",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			// Expectation: New window without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		{
			name:                "background_ctrl-t",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			// Expectation: New tab in existing window.
			expectPreparedTab: true,
			expectNewTabPage:  true,
		},
		{
			name:                "foreground_ctrl-t",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			// Expectation: New tab in existing window.
			expectPreparedTab: true,
			expectNewTabPage:  true,
		},
		{
			name:                "otherdesk_ctrl-t",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			// Expectation: New window (on current desk) without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		// Ctrl-n
		{
			name:                "closed_ctrl-n",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			// Expectation: New window without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		{
			name:                "background_ctrl-n",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			// Expectation: New window without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		{
			name:                "foreground_ctrl-n",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			// Expectation: New window without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		{
			name:                "otherdesk_ctrl-n",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			// Expectation: New window (on current desk) without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
		// ShelfController
		{
			name:                "closed_shelf",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaShelf,
			// Expectation: New window with session restore.
			expectPreparedTab: true,
			expectNewTabPage:  false,
		},
		{
			name:                "background_shelf",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaShelf,
			// Expectation: New tab in existing window.
			expectPreparedTab: true,
			expectNewTabPage:  true,
		},
		{
			name:                "foreground_shelf",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaShelf,
			// Expectation: New tab in existing window.
			expectPreparedTab: true,
			expectNewTabPage:  true,
		},
		{
			name:                "otherdesk_shelf",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaShelf,
			// Expectation: New window (on current desk) without session restore.
			expectPreparedTab: false,
			expectNewTabPage:  true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := prepareBrowser(ctx, cr, browser.TypeLacros, param.browserPrecondition); err != nil {
				s.Fatal("Failed to prepare the browser: ", err)
			}
			if err := param.activateBrowser(ctx, tconn); err != nil {
				s.Fatal("Failed to activate the browser: ", err)
			}
			var expectedTabs []string
			if param.expectPreparedTab {
				expectedTabs = append(expectedTabs, urlForPreparedTab)
			}
			if param.expectNewTabPage {
				expectedTabs = append(expectedTabs, chrome.NewTabURL)
			}
			if err := verifyTabs(ctx, cr, tconn, browser.TypeLacros, expectedTabs); err != nil {
				s.Fatal("Failed to verify browser tabs: ", err)
			}
		})
		if err := cr.ResetState(ctx); err != nil {
			s.Fatal("Failed to reset Chrome: ", err)
		}
	}
}

// activateBrowserViaShelf has the same effect as clicking the browser icon in the launcher.
func activateBrowserViaShelf(ctx context.Context, tconn *chrome.TestConn) error {
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find browser app info")
	}
	if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
		return errors.Wrap(err, "failed to launch browser")
	}
	return nil
}

func activateBrowserViaNewTabShortcut(ctx context.Context, _ *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close()
	if err := kb.Accel(ctx, "Ctrl+t"); err != nil {
		return errors.Wrap(err, "failed to send Ctrl+t")
	}
	return nil
}

func activateBrowserViaNewWindowShortcut(ctx context.Context, _ *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close()
	if err := kb.Accel(ctx, "Ctrl+n"); err != nil {
		return errors.Wrap(err, "failed to send Ctrl+n")
	}
	return nil
}

func verifyTabs(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, bt browser.Type, expectedURLs []string) error {
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return ash.BrowserTypeMatch(bt)(w) && w.IsVisible && w.IsActive && w.HasFocus && w.OnActiveDesk
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to find browser window")
	}
	br, brClose, err := browserfixt.ConnectAndOwn(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to connect to browser")
	}
	defer brClose(ctx)
	tabs, err := br.CurrentTabs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get tabs")
	}
	actualURLs := make([]string, len(tabs))
	for i, tab := range tabs {
		actualURLs[i] = tab.URL
	}
	if !cmp.Equal(actualURLs, expectedURLs) {
		return errors.Errorf("got %v, want %v", actualURLs, expectedURLs)
	}
	return nil
}

func prepareTab(ctx context.Context, cr *chrome.Chrome, bt browser.Type) error {
	conn, _, _, err := browserfixt.SetUpWithURL(ctx, cr, bt, urlForPreparedTab)
	if err != nil {
		return errors.Wrap(err, "failed to set up browser")
	}
	// TODO(crbug.com/1318180): Free the resources when there is a browser-generic way.
	defer conn.Close()
	return nil
}

func prepareBrowser(ctx context.Context, cr *chrome.Chrome, bt browser.Type, browserPrecondition appState) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}
	if err := prepareTab(ctx, cr, bt); err != nil {
		return errors.Wrap(err, "failed to prepare tabs")
	}
	switch browserPrecondition {
	case appStateClosed:
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all windows")
		}
		// TODO(crbug.com/1385579): Get rid of this special handling.
		if bt == browser.TypeLacros {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				info, err := lacrosinfo.Snapshot(ctx, tconn)
				if err != nil {
					return testing.PollBreak(errors.Wrap(err, "failed to get lacros info"))
				}
				if !info.KeepAlive && info.State != lacrosinfo.LacrosStateStopped {
					return errors.Wrap(err, "lacros not yet stopped")
				}
				return nil
			}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
				return errors.Wrap(err, "lacros in unexpected state")
			}
		}
	case appStateBackground:
		window, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
		if err != nil {
			return errors.Wrap(err, "failed to find browser window")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateMinimized); err != nil {
			return errors.Wrap(err, "failed to minimize browser window")
		}
	case appStateForeground:
		// Nothing to do.
	case appStateOtherDesk:
		if err := ash.MoveActiveWindowToAdjacentDesk(ctx, tconn, ash.WindowMovementDirectionLeft); err != nil {
			return errors.Wrap(err, "failed to move browser to other desk")
		}
	}
	return nil
}
