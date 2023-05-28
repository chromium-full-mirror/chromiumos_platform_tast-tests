// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clipboardhistory

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

const clipboardHistoryContextMenuItemName = "Clipboard"
const clipboardHistoryTextItemViewClassName = "ClipboardHistoryTextItemView"
const contextMenuItemViewClassName = "MenuItemView"

// PasteType specifies the action that should trigger a paste from the clipboard
// history menu.
type PasteType int

const (
	// Click pastes entail left-clicking on a menu item.
	Click PasteType = iota
	// Enter pastes entail pressing Enter with a menu item selected.
	Enter
	// Toggle pastes entail toggling the menu closed with a menu item selected.
	Toggle
)

// PasteAndVerify returns an Action that pastes `text` from clipboard history
// into the field specified by `inputField`, replacing whatever text may have
// already been there, and verifies that the paste was successful.
func PasteAndVerify(tconn *chrome.TestConn, ui *uiauto.Context,
	kb *input.KeyboardEventWriter, inputField *nodewith.Finder,
	useContextMenu bool, text string, pasteType PasteType) uiauto.Action {
	return func(ctx context.Context) error {
		// Set input method to US-en so that Ctrl+A behaves as expected.
		ime.EnglishUS.InstallAndActivate(tconn)(ctx)

		clearFieldAction := uiauto.Combine("clear input field",
			ui.LeftClickUntil(inputField, ui.WaitUntilExists(inputField.Focused())),
			kb.AccelAction("Ctrl+A"),
			kb.AccelAction("Backspace"),
			waitForFieldTextToBe(ui, inputField, ""),
		)
		if err := ui.RetrySilently(3, clearFieldAction)(ctx); err != nil {
			return err
		}

		item := nodewith.Name(text).Role(role.MenuItem).HasClass(clipboardHistoryTextItemViewClassName).First()
		if err := uiauto.Combine(fmt.Sprintf("paste %q from clipboard history", text),
			performPaste(ui, kb, inputField, item, text, useContextMenu, pasteType),
			waitForFieldTextToBe(ui, inputField, text),
		)(ctx); err != nil {
			return err
		}

		return nil
	}
}

func waitForFieldTextToBe(ui *uiauto.Context, inputField *nodewith.Finder,
	expectedText string) uiauto.Action {
	return uiauto.Combine("validate field text",
		// Sleep 200ms before validating field text to account for input delay.
		uiauto.Sleep(200*time.Millisecond),
		ui.WithInterval(time.Second).RetrySilently(10, func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, inputField)
			if err != nil {
				return err
			}

			if nodeInfo.Value != expectedText {
				return errors.Errorf("failed to validate input value: got: %s; want: %s", nodeInfo.Value, expectedText)
			}

			return nil
		}))
}

func performPaste(ui *uiauto.Context, kb *input.KeyboardEventWriter,
	inputField, item *nodewith.Finder, text string, useContextMenu bool,
	pasteType PasteType) uiauto.Action {
	return func(ctx context.Context) error {
		// Open clipboard history menu.
		var err error
		if useContextMenu {
			err = uiauto.Combine("opening clipboard history using context menu",
				ui.RightClick(inputField),
				ui.DoDefault(nodewith.NameStartingWith(clipboardHistoryContextMenuItemName).Role(role.MenuItem)),
				ui.WaitUntilGone(nodewith.HasClass(contextMenuItemViewClassName)),
			)(ctx)
		} else {
			err = kb.Accel(ctx, "Search+V")
		}
		if err != nil {
			return err
		}

		// Paste item from menu.
		switch pasteType {
		case Click:
			return ui.LeftClick(item)(ctx)
		case Enter:
			return uiauto.Combine("paste by pressing enter with an item selected",
				// TODO(crbug.com/1385186): Wait until `item` not only exists but also is selected.
				ui.WaitUntilExists(item),
				kb.AccelAction("Enter"),
			)(ctx)
		case Toggle:
			return uiauto.Combine("paste by toggling clipboard history with an item selected",
				// TODO(crbug.com/1385186): Wait until `item` not only exists but also is selected.
				ui.WaitUntilExists(item),
				kb.AccelAction("Search+V"),
			)(ctx)
		}

		return nil
	}
}
