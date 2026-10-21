// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// aimPopupTimeout is how long to wait for the AI Mode popup before
// falling back to the classic omnibox sequence.
const aimPopupTimeout = 5 * time.Second

// AI Mode UI finders, confirmed on R157.
var (
	// aimButtonFinder finds AI Mode buttons.
	aimButtonFinder = nodewith.NameStartingWith("AI Mode").Role(role.Button)
	// aimButtonFinders find the AI Mode button of the active window's
	// focused omnibox: in the Views omnibox, or in the full WebUI popup.
	aimButtonFinders = []*nodewith.Finder{
		aimButtonFinder.HasClass("PageActionView").First(),
		aimButtonFinder.Ancestor(fullPopupFinder).First(),
	}
	// aimPopupFinder finds the AI Mode popup's web view.
	aimPopupFinder = nodewith.HasClass("OmniboxAimPopupWebUIContent").Role(role.WebView).First()
)

// openAimPopup opens the AI Mode popup with the AI Mode button of the
// focused omnibox, whose text is omniboxText, and counts the attempt. With
// modified text, the button submits the text as a search instead of
// opening the popup. So it returns an error without clicking unless
// omniboxText is the unmodified page URL.
func (e *omniboxEnv) openAimPopup(ctx context.Context, omniboxText string) error {
	e.aimAttempts++
	if !e.isUnmodifiedOmniboxText(omniboxText) {
		return errors.Errorf("not clicking AI Mode: omnibox text %q may be modified", omniboxText)
	}
	ui := e.ui.WithTimeout(aimPopupTimeout).WithInterval(200 * time.Millisecond)
	button, err := ui.FindAnyExists(ctx, aimButtonFinders...)
	if err != nil {
		return errors.Wrap(err, "failed to find the AI Mode button")
	}
	if err := uiauto.Combine("open the AI Mode popup",
		e.ui.LeftClick(button),
		ui.WaitUntilExists(aimPopupFinder),
	)(ctx); err != nil {
		return err
	}
	e.aimShown++
	return nil
}

// disableAimIfPopupDisabled turns off the AI Mode step if the WebUI AI
// Mode popup is disabled on the browser command line (e.g. the Views popup
// baseline): the AI Mode button then navigates to Google Search instead of
// opening a popup. Failures are logged.
func (e *omniboxEnv) disableAimIfPopupDisabled(ctx context.Context) {
	p, err := ashproc.RootWithContext(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to find the browser process: ", err)
		return
	}
	args, err := p.CmdlineSliceWithContext(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get the browser command line: ", err)
		return
	}
	if disablesFeature(args, aimPopupFeature) {
		e.disableAim(ctx, aimPopupFeature+" is disabled on the browser command line")
	}
}

// disableAim turns off the AI Mode step for the rest of the run. Desk 3
// then runs the desk-1 sequence.
func (e *omniboxEnv) disableAim(ctx context.Context, reason string) {
	testing.ContextLogf(ctx, "Not using AI Mode for the rest of the run: %s", reason)
	e.aimDisabled = true
}

// revertOmnibox restores the omnibox to the page's unmodified URL. Esc
// doesn't revert typed text, but clearing the text and pressing Esc twice
// does. The omnibox is left unfocused. If the omnibox can't be focused,
// nothing is typed and it returns an error wrapping errOmniboxNotFocused.
func (e *omniboxEnv) revertOmnibox(ctx context.Context) error {
	if _, err := e.focusOmnibox(ctx); err != nil {
		return err
	}
	if err := e.kw.TypeSequence(ctx, []string{"Backspace", "Esc", "Esc"}); err != nil {
		return errors.Wrap(err, "failed to revert the omnibox")
	}
	return nil
}

// aimSequence runs the desk-3 omnibox input in the active window: a short
// classic popup step, then the AI Mode step (aimStep). If the omnibox
// can't be focused for the classic step, it returns an error wrapping
// errOmniboxNotFocused, so that the visit is skipped. After the classic
// step, the visit already had omnibox input, so if the omnibox can't be
// focused again, the visit just ends.
func (e *omniboxEnv) aimSequence(ctx context.Context) error {
	// Classic step. Ctrl+L also shows the on-focus suggestions.
	if _, err := e.focusOmnibox(ctx); err != nil {
		return err
	}
	if err := uiauto.Combine("type in the omnibox and dismiss the popup",
		e.typeAtCadence("omnit"),
		e.kw.AccelAction("Esc"),
	)(ctx); err != nil {
		return err
	}

	err := e.aimStep(ctx)
	if errors.Is(err, errOmniboxNotFocused) {
		testing.ContextLog(ctx, "Ending the visit after the classic step: ", err)
		return nil
	}
	return err
}

// aimStep opens the AI Mode popup and types a prompt without submitting
// it. If the AI Mode popup can't be opened safely or doesn't appear (e.g.
// the account isn't eligible), it runs the desk-1 sequence instead. If the
// AI Mode button navigates, AI Mode is turned off for the rest of the run,
// so the page is loaded at most once.
func (e *omniboxEnv) aimStep(ctx context.Context) error {
	// The AI Mode button is only shown while the omnibox is focused, and
	// reverting leaves it unfocused, so focus it again.
	if err := e.revertOmnibox(ctx); err != nil {
		return err
	}
	titleBefore, err := e.activeWindowTitle(ctx)
	if err != nil {
		return err
	}
	text, err := e.focusOmnibox(ctx)
	if err != nil {
		return err
	}
	if err := e.openAimPopup(ctx, text); err != nil {
		testing.ContextLog(ctx, "Failed to open the AI Mode popup: ", err)
		return e.aimFallback(ctx, titleBefore)
	}

	// Esc closes the popup and moves the prompt into the omnibox.
	if err := uiauto.Combine("type the AI Mode prompt and close the popup",
		e.typeAtCadence(aimPrompt),
		e.kw.AccelAction("Esc"),
	)(ctx); err != nil {
		return err
	}
	if err := e.ui.WithTimeout(aimPopupTimeout).WaitUntilGone(aimPopupFinder)(ctx); err != nil {
		testing.ContextLog(ctx, "AI Mode popup didn't close: ", err)
	}
	// Revert so that the prompt is never submitted later.
	return e.revertOmnibox(ctx)
}

// aimFallback runs the desk-1 sequence when the AI Mode popup couldn't be
// opened. The account may be ineligible, or the button may navigate to an
// empty AI Mode search instead (as it does in some sessions, regardless of
// the omnibox text, or with AiModeEntryPointAlwaysNavigates); in that case,
// it goes back to the page with title titleBefore and turns off AI Mode for
// the rest of the run.
func (e *omniboxEnv) aimFallback(ctx context.Context, titleBefore string) error {
	if err := e.kw.Accel(ctx, "Esc"); err != nil {
		return err
	}
	if titleAfter, err := e.activeWindowTitle(ctx); err == nil && titleAfter != titleBefore {
		testing.ContextLogf(ctx, "AI Mode button navigated (%q -> %q); going back", titleBefore, titleAfter)
		e.disableAim(ctx, "the AI Mode button navigates instead of opening the popup")
		if err := e.kw.Accel(ctx, "Alt+Left"); err != nil {
			return errors.Wrap(err, "failed to go back")
		}
		if err := e.waitForActiveWindowTitle(ctx, titleBefore, omniboxPageLoadTimeout); err != nil {
			testing.ContextLog(ctx, "Failed to wait for the original page after going back: ", err)
		}
	}
	return e.classicSequence(ctx)
}

// recordAimResults saves Omnibox.AimAvailable, the fraction of desk-3
// visits where the AI Mode popup was shown.
func (e *omniboxEnv) recordAimResults(ctx context.Context, pv *perf.Values) {
	if e.aimAttempts > 0 {
		avail := float64(e.aimShown) / float64(e.aimAttempts)
		pv.Set(perf.Metric{
			Name:      "Omnibox.AimAvailable",
			Unit:      "ratio",
			Direction: perf.BiggerIsBetter,
		}, avail)
		testing.ContextLogf(ctx, "AI Mode popup shown on %d of %d desk-3 visits", e.aimShown, e.aimAttempts)
	}
}
