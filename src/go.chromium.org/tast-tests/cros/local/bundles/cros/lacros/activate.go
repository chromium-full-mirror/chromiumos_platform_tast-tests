// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Activate,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests browser activation via shelf controller and via accelerator shortcuts",
		Contacts:     []string{"lacros-team@google.com", "neis@google.com"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "lacros", "gpu_sandboxing"},
		Params: []testing.Param{{
			Name:      "no_keep_alive",
			Fixture:   "lacros",
			ExtraAttr: []string{},
		}, {
			Name:    "keep_alive",
			Fixture: "lacrosKeepAlive",
		}},
		Timeout: 5 * time.Minute,
	})
}

type appState string

const (
	appStateClosed     appState = "Closed"
	appStateBackground appState = "Background"
	appStateForeground appState = "Foreground"
	appStateOtherDesk  appState = "OtherDesk"
)

type behavior string

const (
	behaviorNewTabInExistingWindow         behavior = "NewTabInExistingWindow"
	behaviorNewWindowWithoutSessionRestore behavior = "NewWindowWithoutSessionRestore"
	behaviorNewWindowWithSessionRestore    behavior = "NewWindowWithSessionRestore"
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
		expectation         behavior
	}{
		// Ctrl-t
		{
			name:                "closed_ctrl-t",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore,
		},
		{
			name:                "background_ctrl-t",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			expectation:         behaviorNewTabInExistingWindow,
		},
		{
			name:                "foreground_ctrl-t",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			expectation:         behaviorNewTabInExistingWindow,
		},
		{
			name:                "otherdesk_ctrl-t",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaNewTabShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore,
		},
		// Ctrl-n
		{
			name:                "closed_ctrl-n",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore,
		},
		{
			name:                "background_ctrl-n",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore,
		},
		{
			name:                "foreground_ctrl-n",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore,
		},
		{
			name:                "otherdesk_ctrl-n",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaNewWindowShortcut,
			expectation:         behaviorNewWindowWithoutSessionRestore, // (on current desk)
		},
		// ShelfController
		{
			name:                "closed_shelf",
			browserPrecondition: appStateClosed,
			activateBrowser:     activateBrowserViaShelf,
			expectation:         behaviorNewWindowWithSessionRestore,
		},
		{
			name:                "background_shelf",
			browserPrecondition: appStateBackground,
			activateBrowser:     activateBrowserViaShelf,
			expectation:         behaviorNewTabInExistingWindow,
		},
		{
			name:                "foreground_shelf",
			browserPrecondition: appStateForeground,
			activateBrowser:     activateBrowserViaShelf,
			expectation:         behaviorNewTabInExistingWindow,
		},
		{
			name:                "otherdesk_shelf",
			browserPrecondition: appStateOtherDesk,
			activateBrowser:     activateBrowserViaShelf,
			expectation:         behaviorNewWindowWithoutSessionRestore, // (on current desk)
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.SaveScreenshotToFileOnError(ctx, cr, s.OutDir(), s.HasError, param.name+".png")

			initialWindow, err := prepareBrowser(ctx, cr, browser.TypeLacros, param.browserPrecondition)
			if err != nil {
				s.Fatal("Failed to prepare the browser: ", err)
			}
			if err := param.activateBrowser(ctx, tconn); err != nil {
				s.Fatal("Failed to activate the browser: ", err)
			}
			var expectedTabs []string
			if param.expectation != behaviorNewWindowWithoutSessionRestore {
				expectedTabs = append(expectedTabs, urlForPreparedTab)
			}
			if param.expectation != behaviorNewWindowWithSessionRestore {
				expectedTabs = append(expectedTabs, chrome.NewTabURL)
			}
			expectNewWindow := param.expectation != behaviorNewTabInExistingWindow
			if err := verifyTabs(ctx, cr, tconn, browser.TypeLacros, initialWindow, expectNewWindow, expectedTabs); err != nil {
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
	defer kb.Close(ctx)
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
	defer kb.Close(ctx)
	if err := kb.Accel(ctx, "Ctrl+n"); err != nil {
		return errors.Wrap(err, "failed to send Ctrl+n")
	}
	return nil
}

func verifyTabs(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, bt browser.Type, initialWindow *ash.Window, expectNewWindow bool, expectedURLs []string) error {
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return (initialWindow == nil || (w.ID != initialWindow.ID) == expectNewWindow) && ash.BrowserTypeMatch(bt)(w) && w.IsVisible && w.IsActive && w.HasFocus && w.OnActiveDesk
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to find browser window")
	}

	br, brClose, err := browserfixt.ConnectAndOwn(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to connect to browser")
	}
	defer brClose(ctx)

	return testing.Poll(ctx, func(ctx context.Context) error {
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
	}, &testing.PollOptions{Timeout: 5 * time.Second})
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

func prepareBrowser(ctx context.Context, cr *chrome.Chrome, bt browser.Type, browserPrecondition appState) (*ash.Window, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API")
	}

	if err := prepareTab(ctx, cr, bt); err != nil {
		return nil, errors.Wrap(err, "failed to prepare tabs")
	}

	if browserPrecondition == appStateClosed {
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to close all windows")
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
				return nil, errors.Wrap(err, "lacros in unexpected state")
			}
		}
		return nil, nil
	}

	window, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find browser window")
	}

	switch browserPrecondition {
	case appStateBackground:
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateMinimized); err != nil {
			return nil, errors.Wrap(err, "failed to minimize browser window")
		}
	case appStateForeground:
		// Nothing to do.
	case appStateOtherDesk:
		if err := ash.MoveActiveWindowToAdjacentDesk(ctx, tconn, ash.WindowMovementDirectionLeft); err != nil {
			return nil, errors.Wrap(err, "failed to move browser to other desk")
		}
		window, err = ash.FindOnlyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
		if err != nil {
			return nil, errors.Wrap(err, "failed to find browser window")
		}

	}
	return window, nil
}
