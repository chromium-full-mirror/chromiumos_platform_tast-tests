// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// omniboxNewTabTitle is the title of the New Tab page, which is in
	// the window title after Ctrl+T.
	omniboxNewTabTitle = "New Tab"

	// omniboxNewTabTimeout is how long to wait for the New Tab page after
	// Ctrl+T.
	omniboxNewTabTimeout = 10 * time.Second

	// omniboxPageLoadTimeout is how long to wait for a page navigated to
	// via the omnibox.
	omniboxPageLoadTimeout = 30 * time.Second
)

// classicSequence runs the desk-1 sequence: Ctrl+L; type a corpus string;
// Down, Down; Esc, Esc.
func (e *omniboxEnv) classicSequence(ctx context.Context) error {
	if _, err := e.focusOmnibox(ctx); err != nil {
		return err
	}
	return uiauto.Combine("type in the omnibox and dismiss the popup",
		e.typeAtCadence(e.nextCorpus()),
		e.kw.TypeSequenceAction([]string{"Down", "Down", "Esc", "Esc"}),
	)(ctx)
}

// navigateViaOmnibox types the URL of page n into the focused omnibox,
// presses Enter, and waits for the page.
func (e *omniboxEnv) navigateViaOmnibox(ctx context.Context, n int) error {
	if err := uiauto.Combine(fmt.Sprintf("type the URL of page %d and press Enter", n),
		e.typeAtCadence(e.typedPageURL(n)),
		e.kw.AccelAction("Enter"),
	)(ctx); err != nil {
		return err
	}
	if err := e.waitForActiveWindowTitle(ctx, omniboxPageTitle(n), omniboxPageLoadTimeout); err != nil {
		return errors.Wrapf(err, "failed to wait for page %d", n)
	}
	return nil
}

// desk1Action returns the desk-1 action: the classic popup sequence in
// the active window.
func (e *omniboxEnv) desk1Action() action.Action {
	return e.visitAction(e.classicSequence)
}

// desk2Action returns the desk-2 action: activate the next window on the
// desk, then the desk-1 sequence. The first rotation shows popups that
// were never shown; later visits re-show them after a long idle.
func (e *omniboxEnv) desk2Action() action.Action {
	return e.visitAction(uiauto.Combine("use the omnibox in the next window",
		e.activateNextWindowOnDesk(&e.desk2Idx),
		e.classicSequence,
	))
}

// desk3Action returns the desk-3 action: alternate between the desk's
// windows, then the AI Mode sequence (aimSequence).
func (e *omniboxEnv) desk3Action() action.Action {
	return e.visitAction(func(ctx context.Context) error {
		// With AI Mode turned off (e.g. the Views popup baseline), desk 3
		// runs the desk-1 sequence.
		input := e.aimSequence
		if e.aimDisabled {
			input = e.classicSequence
		}
		return uiauto.Combine("use the omnibox in the next window",
			e.activateNextWindowOnDesk(&e.desk3Idx),
			input,
		)(ctx)
	})
}

// desk4Action returns the desk-4 action: open a new tab, navigate to a
// local page via the omnibox, then close the tab. The desk's window and
// its page are never navigated.
func (e *omniboxEnv) desk4Action() action.Action {
	return e.visitAction(func(ctx context.Context) error {
		n := omniboxPagesNav[e.navIdx%len(omniboxPagesNav)]
		e.navIdx++

		if err := e.kw.Accel(ctx, "Ctrl+T"); err != nil {
			return errors.Wrap(err, "failed to open a new tab")
		}
		// Only close the tab with Ctrl+W if the new tab was opened.
		// Otherwise Ctrl+W would close the desk's only tab, and with it
		// the window. Desk 4 has one window, and with the full WebUI
		// popup the popup is the active window, so don't require the
		// browser window to be active.
		if err := e.waitForWindowTitleOnActiveDesk(ctx, omniboxNewTabTitle, omniboxNewTabTimeout); err != nil {
			return errors.Wrap(errOmniboxNotFocused, "the new tab wasn't opened: "+err.Error())
		}
		_, inputErr := e.focusOmnibox(ctx)
		if inputErr == nil {
			inputErr = e.navigateViaOmnibox(ctx, n)
		}
		// Close the new tab, even if the visit is skipped.
		if err := e.kw.Accel(ctx, "Ctrl+W"); err != nil {
			return errors.Wrap(err, "failed to close the tab")
		}
		return inputErr
	})
}

// onVisitActions returns the per-desk omnibox actions.
func (e *omniboxEnv) onVisitActions() []action.Action {
	return []action.Action{e.desk1Action(), e.desk2Action(), e.desk3Action(), e.desk4Action()}
}

// openWindowsViaOmnibox opens a window for each page in
// omniboxPagesRecorded on the active desk, via Ctrl+N and the omnibox. It
// records the time from Ctrl+N to the new window being visible, as a
// test-side fallback for Ash.Window.InitToFirstPresentation.Browser.
// Returns the URL of the last opened page.
func (e *omniboxEnv) openWindowsViaOmnibox(ctx context.Context) (string, error) {
	windows, err := ash.GetAllWindows(ctx, e.tconn)
	if err != nil {
		return "", errors.Wrap(err, "failed to get windows")
	}
	known := make(map[int]bool)
	for _, w := range windows {
		known[w.ID] = true
	}

	for _, n := range omniboxPagesRecorded {
		start := time.Now()
		if err := e.kw.Accel(ctx, "Ctrl+N"); err != nil {
			return "", errors.Wrap(err, "failed to open a new window")
		}
		if err := ash.WaitForCondition(ctx, e.tconn, func(w *ash.Window) bool {
			return !known[w.ID] && w.IsVisible && w.IsActive
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 10 * time.Millisecond}); err != nil {
			return "", errors.Wrap(err, "failed to wait for the new window")
		}
		e.newWindowLatencies = append(e.newWindowLatencies, time.Since(start))

		// Navigate away from the NTP immediately.
		if _, err := e.focusOmnibox(ctx); err != nil {
			return "", err
		}
		if err := e.navigateViaOmnibox(ctx, n); err != nil {
			return "", err
		}
		w, err := ash.GetActiveWindow(ctx, e.tconn)
		if err != nil {
			return "", err
		}
		known[w.ID] = true
		if err := ash.SetWindowStateAndWait(ctx, e.tconn, w.ID, ash.WindowStateMaximized); err != nil {
			return "", errors.Wrap(err, "failed to maximize the new window")
		}
	}
	return e.pageURL(omniboxPagesRecorded[len(omniboxPagesRecorded)-1]), nil
}

// navigateToVersionURL navigates the page at url to chrome://version, to
// record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2 as the
// default variant does with Google Slides. Failures are logged.
func (e *omniboxEnv) navigateToVersionURL(ctx context.Context, url string) {
	conn, err := e.cr.NewConnForTarget(ctx, chrome.MatchTargetURL(url))
	if err != nil {
		testing.ContextLog(ctx, "Failed to connect to the last omnibox-opened page: ", err)
		return
	}
	defer conn.Close()
	if err := conn.Navigate(ctx, chrome.VersionURL); err != nil {
		testing.ContextLogf(ctx, "Failed to navigate to %s: %v", chrome.VersionURL, err)
	}
}
