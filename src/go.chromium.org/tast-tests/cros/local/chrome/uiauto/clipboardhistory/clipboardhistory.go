// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clipboardhistory

import (
	"context"
	"fmt"
	"strings"
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
const clipboardHistorySubmenuItemName = "Paste from clipboard"
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

// PasteSource indicates the source of the pasted clipboard history data.
type PasteSource int

const (
	// ClipboardHistoryMenuFromContextMenu indicates the standalone clipboard
	// history menu opened from a context menu. NOTE: This source is available
	// only when clipboard history refresh feature is disabled.
	ClipboardHistoryMenuFromContextMenu PasteSource = iota

	// ClipboardHistoryMenuFromContextMenuSubmenu indicates the standalone clipboard
	// history menu opened from a context menu submenu. NOTE: This source is available
	// only when clipboard history refresh feature is enabled.
	ClipboardHistoryMenuFromContextMenuSubmenu

	// ClipboardHistoryMenuFromShortcut indicates the standalone clipboard history menu
	// opened by the keyboard shortcut.
	ClipboardHistoryMenuFromShortcut

	// ClipboardHistorySubmenu indicates the clipboard history submenu. NOTE: This
	// source is available only when clipboard history refresh feature is enabled.
	ClipboardHistorySubmenu
)

// PasteAndVerify returns an Action that pastes `text` from clipboard history
// into the field specified by `inputField`, replacing whatever text may have
// already been there, and verifies that the paste was successful. `source`
// indicates the source of clipboard history data.
func PasteAndVerify(tconn *chrome.TestConn, ui *uiauto.Context,
	kb *input.KeyboardEventWriter, inputField *nodewith.Finder,
	source PasteSource, text string, pasteType PasteType) uiauto.Action {
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

		item := nodewith.Name(text).Role(role.MenuItem).First()
		switch source {
		case ClipboardHistorySubmenu:
			// Specify that `item` is in the clipboard history submenu.
			item = item.Ancestor(nodewith.Name(clipboardHistorySubmenuItemName).Role(role.Menu))
		case ClipboardHistoryMenuFromShortcut:
		case ClipboardHistoryMenuFromContextMenu:
		case ClipboardHistoryMenuFromContextMenuSubmenu:
			// Specify that `item` is in the standalone clipboard history menu.
			item = item.HasClass(clipboardHistoryTextItemViewClassName)
		}

		if err := uiauto.Combine(fmt.Sprintf("paste %q from clipboard history", text),
			performPaste(ui, kb, inputField, item, source, pasteType),
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

			if !strings.Contains(strings.TrimSpace(nodeInfo.Value), expectedText) {
				return errors.Errorf("failed to validate input value: got: %q; want: %q", nodeInfo.Value, expectedText)
			}

			return nil
		}))
}

func performPaste(ui *uiauto.Context, kb *input.KeyboardEventWriter,
	inputField, item *nodewith.Finder, source PasteSource,
	pasteType PasteType) uiauto.Action {
	return func(ctx context.Context) error {
		if source == ClipboardHistorySubmenu && pasteType == Toggle {
			return errors.New("failed to perform paste: clipboard history submenu does not support toggle pastes")
		}

		// Show `source`.
		var err error
		switch source {
		case ClipboardHistoryMenuFromShortcut:
			err = uiauto.Combine("opening the standalone clipboard history menu using the accelerator",
				kb.AccelAction("Search+V"))(ctx)
		case ClipboardHistoryMenuFromContextMenu:
			err = uiauto.Combine("opening the standalone clipboard history menu from the context menu",
				ui.RightClick(inputField),
				ui.DoDefault(nodewith.NameStartingWith(clipboardHistoryContextMenuItemName).Role(role.MenuItem)),
				ui.WaitUntilGone(nodewith.HasClass(contextMenuItemViewClassName)),
			)(ctx)
		case ClipboardHistoryMenuFromContextMenuSubmenu:
			fallthrough
		case ClipboardHistorySubmenu:
			err = uiauto.Combine("opening the clipboard history submenu",
				ui.RightClick(inputField),
				ui.LeftClick(nodewith.NameStartingWith(clipboardHistorySubmenuItemName).Role(role.MenuItem)),
			)(ctx)
			if err != nil {
				return err
			}
			if source == ClipboardHistoryMenuFromContextMenuSubmenu {
				err = uiauto.Combine("opening the standalone clipboard history menu from submenu",
					ui.DoDefault(nodewith.NameStartingWith(clipboardHistoryContextMenuItemName).Role(role.MenuItem)),
					ui.WaitUntilGone(nodewith.HasClass(contextMenuItemViewClassName)),
				)(ctx)
				if err != nil {
					return err
				}
			}
		}

		if err != nil {
			return err
		}

		// Paste `item`.
		switch pasteType {
		case Click:
			return uiauto.Combine("paste by left-clicking the item",
				ui.WaitUntilExists(item),
				ui.LeftClick(item))(ctx)
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
