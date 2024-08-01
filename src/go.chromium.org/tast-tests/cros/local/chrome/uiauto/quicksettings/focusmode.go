// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// focusModeDetailedView is the detailed Focus Mode view within the Quick
// Settings.
var focusModeDetailedView = nodewith.ClassNameRegex(regexp.MustCompile(`^FocusModeDetailedView[A-Za-z]*$`))

// FocusModeDetailedViewStartButton is the "Start" button child within the
// detailed Focus Mode view.
var FocusModeDetailedViewStartButton = nodewith.HasClass("PillButton").NameContaining("Start").Ancestor(focusModeDetailedView)

// FocusModeDetailedViewFinishButton is the "Finish" button child within the
// detailed Focus Mode view.
var FocusModeDetailedViewFinishButton = nodewith.HasClass("PillButton").NameContaining("Finish").Ancestor(focusModeDetailedView)

// FocusModeDetailedViewTimerTextfield is the minutes textfield child within the
// detailed Focus Mode view.
var FocusModeDetailedViewTimerTextfield = nodewith.HasClass("SystemTextfield").NameContaining("Edit timer.").Ancestor(focusModeDetailedView)

// NavigateToFocusModeDetailedView will navigate to the detailed Focus Mode view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func NavigateToFocusModeDetailedView(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// The Quick Settings may be auto-collapsed after being expanded due to notifications or other
	// events so we continue attempting to navigate to the Focus Mode page for up to one minute.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := Show(ctx, tconn); err != nil {
			return err
		}
		return uiauto.Combine("Click the Focus Mode feature tile",
			ui.DoDefault(FeatureTileFocusMode),
			ui.WaitUntilExists(focusModeDetailedView),
		)(ctx)
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second})
}

// EnsureFocusModeHasStarted will navigate to the detailed Focus Mode view within the
// Quick Settings, then attempt to start a Focus session. We ensure the duration will
// be greater than the test duration by setting it to the maximum value of "300".
// It does nothing if there is already an active Focus session.
func EnsureFocusModeHasStarted(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	if err := NavigateToFocusModeDetailedView(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to navigate to the detailed Focus Mode view")
	}
	defer Hide(ctx, tconn)

	if finishButtonFound, err := ui.IsNodeFound(ctx, FocusModeDetailedViewFinishButton); err != nil {
		return err
	} else if finishButtonFound {
		// No action required if there is already an active Focus session.
		return nil
	}

	// Select the Timer Textfield.
	if err := ui.DoDefault(FocusModeDetailedViewTimerTextfield)(ctx); err != nil {
		return errors.Wrap(err, "failed to focus the timer textfield")
	}

	// Open a connection to the keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	// // Enter the maximum time duration of "300" minutes.
	if err := kb.TypeAction("300")(ctx); err != nil {
		return errors.Wrap(err, "failed to enter session duration")
	}

	if err := ui.DoDefault(FocusModeDetailedViewStartButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to start a Focus session")
	}

	return nil
}

// EnsureFocusModeEnds will navigate to the detailed Focus Mode view within the
// Quick Settings, then attempt to end the Focus session. Returns an error if there is no active Focus session.
func EnsureFocusModeEnds(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	if err := NavigateToFocusModeDetailedView(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to navigate to the detailed Focus Mode view")
	}
	defer Hide(ctx, tconn)

	if startButtonFound, err := ui.IsNodeFound(ctx, FocusModeDetailedViewStartButton); err != nil {
		return err
	} else if startButtonFound {
		return errors.Errorf("focus mode was terminated before test ended")
	}

	if err := ui.DoDefault(FocusModeDetailedViewFinishButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to end the Focus session")
	}

	return nil
}
