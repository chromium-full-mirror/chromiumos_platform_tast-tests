// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dictation

import (
	"context"
	"regexp"
	"time"

	dictationcommon "go.chromium.org/tast-tests/cros/common/dictation"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	incognitoTab = nodewith.Name("New Incognito Tab").Role(role.Tab)
	// TODO(b/444070886): Clean up the "BrowserFrame" naming after the Chrome uprev contains the new naming.
	settingsWindow      = nodewith.Name("Settings").Role(role.Window).ClassNameRegex(regexp.MustCompile("Browser(Widget|Frame)"))
	closeSettingsButton = nodewith.Name("Close").Role(role.Button).Ancestor(settingsWindow)
)

// PrepareKeyboardEvent prepares the precondition for a keyboard event.
func (s *Support) PrepareKeyboardEvent(ctx context.Context, event string) error {
	ui := s.ui
	switch event {
	case dictationcommon.ButtonEndOfLetterPriority:
		testing.ContextLog(ctx, "Focus on 'setEventMode' button")
		if err := ui.FocusAndWait(setEventModeButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to focus on 'setEventMode' button")
		}
	case dictationcommon.ButtonInsertOverwrite:
		testing.ContextLog(ctx, "Focus on 'getEventMode' button")
		if err := ui.FocusAndWait(getEventModeButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to focus on 'getEventMode' button")
		}
	case dictationcommon.ButtonForward:
		// No action required.
	case dictationcommon.ButtonInstruction:
		testing.ContextLog(ctx, "Click on search bar")
		searchBar := nodewith.Name("Address and search bar").Role(role.TextField)
		if err := ui.DoDefault(searchBar)(ctx); err != nil {
			return errors.Wrap(err, "failed to click search bar")
		}
	case dictationcommon.FunctionF1A:
		testing.ContextLog(ctx, "Open the second tab for opening Bookmark window")
		conn, err := s.cr.NewConn(ctx, "https://www.google.com")
		if err != nil {
			return errors.Wrap(err, "failed to open a new Google tab")
		}
		defer conn.Close()
	case dictationcommon.FunctionF2B:
		settingsButton := nodewith.Name("Settings").Role(role.Button).HasClass("ShelfAppButton")
		if err := uiauto.NamedCombine("open and close settings window",
			ui.DoDefault(settingsButton),
			ui.DoDefault(closeSettingsButton),
		)(ctx); err != nil {
			return err
		}
	default:
		return errors.Errorf("unrecognized event %v", event)
	}
	return nil
}

// VerifyKeyboardEvent verifies that the given keyboard event is triggered.
func (s *Support) VerifyKeyboardEvent(ctx context.Context, event string) error {
	ui := s.ui
	switch event {
	case dictationcommon.ButtonEndOfLetterPriority:
		// Pressing 'End-of-letter/Priority' button should act as tab backward,
		// equal to shift + tab on the keyboard.
		// After pressing 'End-of-letter/Priority' button, it should focused on
		// 'getEventMode' button.
		if err := ui.WaitUntilExists(getEventModeButton.Focused())(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for focusing on 'getEventMode' button")
		}
	case dictationcommon.ButtonInsertOverwrite:
		// Pressing 'Insert/Overwrite' button should act as tab forward,
		// equal to tab on the keyboard.
		// After pressing 'Insert/Overwrite' button, it should focused on
		// 'setEventMode' button.
		if err := ui.WaitUntilExists(setEventModeButton.Focused())(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for focusing on 'setEventMode' button")
		}
	case dictationcommon.ButtonForward:
		// Pressing 'Fast-Forward' button should open a new window,
		// equal to ctrl + shift + n on the keyboard.
		// After opening a new window, it should displays incognito tab.
		if err := ui.WaitUntilExists(incognitoTab)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for incognito tab")
		}
	case dictationcommon.ButtonInstruction:
		// Pressing 'Instruction' button should type '+', equal to pressing
		// Numkey + on keyboard.
		// After pressing '+', the list box option "+ search" will be displayed.
		plusText := nodewith.Name("+ search").Role(role.ListBoxOption)
		if err := ui.WaitUntilExists(plusText)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for '+ search'")
		}
	case dictationcommon.FunctionF1A:
		// Pressing 'F1' button should open Bookmark window, equal to
		// ctrl + shift + d on keyboard.
		bookMarkWindow := nodewith.Name("Bookmark all tabs").Role(role.Window)
		if err := ui.WaitUntilExists(bookMarkWindow)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the Bookmark window")
		}
	case dictationcommon.FunctionF2B:
		// Pressing 'F2' button should open Settings window, equal to
		// ctrl + shift + t on keyboard.
		if err := ui.WaitUntilExists(settingsWindow)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the settings window")
		}
	default:
		return errors.Errorf("unrecognized event %v", event)
	}
	testing.ContextLogf(ctx, "Successfully verified the %q keyboard event", event)
	return nil
}

// CleanupKeyboardEvent cleans up by performing necessary actions like
// closing windows.
func (s *Support) CleanupKeyboardEvent(ctx context.Context, event string) error {
	ui := s.ui
	switch event {
	case dictationcommon.ButtonEndOfLetterPriority,
		dictationcommon.ButtonInsertOverwrite,
		dictationcommon.ButtonInstruction:
		// No cleanup required.
	case dictationcommon.ButtonForward:
		testing.ContextLog(ctx, "Close incognito tab")
		closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(incognitoTab)
		return ui.DoDefaultUntil(closeButton,
			ui.WithTimeout(time.Second).WaitUntilGone(closeButton),
		)(ctx)
	case dictationcommon.FunctionF1A:
		cancelButton := nodewith.Name("Cancel").Role(role.Button)
		googleTab := nodewith.Name("Google").Role(role.Tab)
		closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(googleTab)
		return uiauto.NamedCombine("cancel bookmark and close google tab",
			ui.DoDefault(cancelButton),
			ui.DoDefaultUntil(closeButton, ui.WithTimeout(time.Second).WaitUntilGone(closeButton)),
		)(ctx)
	case dictationcommon.FunctionF2B:
		testing.ContextLog(ctx, "Close settings window")
		return uiauto.NamedAction("close settings window",
			ui.DoDefault(closeSettingsButton),
		)(ctx)
	default:
		return errors.Errorf("unrecognized event %v", event)
	}
	return nil
}
