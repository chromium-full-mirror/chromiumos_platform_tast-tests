// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"sort"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// omniboxTypingInterval is the fixed per-character typing cadence.
	omniboxTypingInterval = 150 * time.Millisecond

	// omniboxFocusTimeout is how long to wait for the omnibox to be
	// focused after Ctrl+L, per attempt.
	omniboxFocusTimeout = 10 * time.Second

	// omniboxFocusAttempts is how many times to press Ctrl+L before
	// giving up on focusing the omnibox.
	omniboxFocusAttempts = 2
)

// errOmniboxNotFocused is returned by focusOmnibox when the omnibox
// couldn't be focused. Desk actions return it (possibly wrapped) to skip
// the visit; see visitAction.
var errOmniboxNotFocused = errors.New("omnibox not focused")

// Omnibox input finders, confirmed on R157.
var (
	// fullPopupFinder finds the focused full WebUI popup
	// (WebUIOmniboxFullPopup). With it, the focused omnibox input is
	// drawn by the popup's WebUI instead of the Views omnibox. Every
	// window has one, so match the focused one.
	fullPopupFinder = nodewith.HasClass("OmniboxFullPopupWebUIContent").Role(role.WebView).Focused()

	// focusedOmniboxFinders find the focused omnibox input of the active
	// window: the Views omnibox, or the full WebUI popup's input.
	focusedOmniboxFinders = []*nodewith.Finder{
		browserui.AddressBarFinder.Focused(),
		nodewith.Role(role.TextFieldWithComboBox).Ancestor(fullPopupFinder).Focused(),
	}
)

// corpus returns the fixed strings typed into the omnibox. They
// prefix-match the seeded page titles and URLs. They are never submitted.
func (e *omniboxEnv) corpus() []string {
	return []string{
		"omnit",
		"omnitest 0",
		"omnitest 1",
		e.hostPort + "/p0",
		"omnitest 0",
		e.hostPort + "/p1",
	}
}

// nextCorpus returns the next string from the corpus.
func (e *omniboxEnv) nextCorpus() string {
	c := e.corpus()
	s := c[e.corpusIdx%len(c)]
	e.corpusIdx++
	return s
}

// typeAtCadence returns an action that types s one character at a time at
// omniboxTypingInterval.
func (e *omniboxEnv) typeAtCadence(s string) action.Action {
	return func(ctx context.Context) error {
		for _, r := range s {
			start := time.Now()
			if err := e.kw.Type(ctx, string(r)); err != nil {
				return errors.Wrapf(err, "failed to type %q", r)
			}
			if remaining := omniboxTypingInterval - time.Since(start); remaining > 0 {
				// GoBigSleepLint: Keeps a fixed typing cadence.
				if err := testing.Sleep(ctx, remaining); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// focusOmnibox presses Ctrl+L, waits until the omnibox of the active
// window is focused, and returns its text. Under heavy memory pressure,
// the omnibox sometimes isn't focused after Ctrl+L, so it tries
// omniboxFocusAttempts times. It returns an error wrapping
// errOmniboxNotFocused if the omnibox still isn't focused; the caller
// must not type then, since the keys would go to the web page.
func (e *omniboxEnv) focusOmnibox(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= omniboxFocusAttempts; attempt++ {
		if err := e.kw.Accel(ctx, "Ctrl+L"); err != nil {
			return "", errors.Wrap(err, "failed to press Ctrl+L")
		}
		v, err := e.focusedOmniboxValue(ctx)
		if err == nil {
			return v, nil
		}
		testing.ContextLogf(ctx, "Omnibox not focused after Ctrl+L (attempt %d): %v", attempt, err)
		lastErr = err
	}
	return "", errors.Wrap(errOmniboxNotFocused, lastErr.Error())
}

// focusedOmniboxValue waits up to omniboxFocusTimeout for a focused omnibox
// input (see focusedOmniboxFinders) and returns its text.
func (e *omniboxEnv) focusedOmniboxValue(ctx context.Context) (string, error) {
	ui := e.ui.WithTimeout(omniboxFocusTimeout).WithInterval(200 * time.Millisecond)
	finder, err := ui.FindAnyExists(ctx, focusedOmniboxFinders...)
	if err != nil {
		return "", errors.Wrap(err, "no focused omnibox input")
	}
	info, err := ui.Info(ctx, finder)
	if err != nil {
		return "", errors.Wrap(err, "failed to get the omnibox text")
	}
	return info.Value, nil
}

// visitAction returns an action that runs the desk action a as one visit
// and counts it. If a returns an error wrapping errOmniboxNotFocused, the
// visit is counted as skipped and the action returns nil.
func (e *omniboxEnv) visitAction(a action.Action) action.Action {
	return func(ctx context.Context) error {
		e.visits++
		err := a(ctx)
		if errors.Is(err, errOmniboxNotFocused) {
			testing.ContextLog(ctx, "Skipping the omnibox input on this visit: ", err)
			e.skippedVisits++
			return nil
		}
		return err
	}
}

// checkSkippedVisits returns an error if the omnibox input was skipped on
// more than half of the visits, e.g. because the omnibox finder doesn't
// match the omnibox in this configuration. Otherwise the run would pass
// without exercising the omnibox.
func (e *omniboxEnv) checkSkippedVisits() error {
	if e.visits > 0 && e.skippedVisits*2 > e.visits {
		return errors.Errorf("the omnibox couldn't be focused on %d of %d visits", e.skippedVisits, e.visits)
	}
	return nil
}

// activateNextWindowOnDesk returns an action that activates the window
// after *idx among the windows on the active desk, sorted by window ID, and
// advances *idx.
func (e *omniboxEnv) activateNextWindowOnDesk(idx *int) action.Action {
	return func(ctx context.Context) error {
		ws, err := ash.FindAllWindows(ctx, e.tconn, func(w *ash.Window) bool { return w.OnActiveDesk })
		if err != nil {
			return errors.Wrap(err, "failed to get the windows on the active desk")
		}
		if len(ws) == 0 {
			return errors.New("no windows on the active desk")
		}
		sort.Slice(ws, func(i, j int) bool { return ws[i].ID < ws[j].ID })
		*idx = (*idx + 1) % len(ws)
		w := ws[*idx]
		if err := w.ActivateWindow(ctx, e.tconn); err != nil {
			return errors.Wrapf(err, "failed to activate window %d", w.ID)
		}
		return ash.WaitForCondition(ctx, e.tconn, func(win *ash.Window) bool {
			return win.ID == w.ID && win.IsActive
		}, &testing.PollOptions{Timeout: 10 * time.Second})
	}
}

// activeWindowTitle returns the title of the active window.
func (e *omniboxEnv) activeWindowTitle(ctx context.Context) (string, error) {
	w, err := ash.GetActiveWindow(ctx, e.tconn)
	if err != nil {
		return "", err
	}
	return w.Title, nil
}

// waitForActiveWindowTitle waits until the active window's title contains
// want. Browser window titles look like "Chrome - <page title>".
func (e *omniboxEnv) waitForActiveWindowTitle(ctx context.Context, want string, timeout time.Duration) error {
	return ash.WaitForCondition(ctx, e.tconn, func(w *ash.Window) bool {
		return w.IsActive && strings.Contains(w.Title, want)
	}, &testing.PollOptions{Timeout: timeout})
}

// waitForWindowTitleOnActiveDesk waits until a window on the active desk
// has a title that contains want.
func (e *omniboxEnv) waitForWindowTitleOnActiveDesk(ctx context.Context, want string, timeout time.Duration) error {
	return ash.WaitForCondition(ctx, e.tconn, func(w *ash.Window) bool {
		return w.OnActiveDesk && strings.Contains(w.Title, want)
	}, &testing.PollOptions{Timeout: timeout})
}
