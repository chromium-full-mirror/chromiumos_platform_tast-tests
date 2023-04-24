// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imetestutil provides shared struct to test IME behaviour in Crostini.
package imetestutil

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ime/emojipicker"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
)

// OpenEmojiPickerAndInputEmoji opens the emoji picker using the keyboard shortcut and selects the emojiChar.
func OpenEmojiPickerAndInputEmoji(ctx context.Context, cr *chrome.Chrome, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, emojiChar string) uiauto.Action {
	emojiCharFinder := emojipicker.NodeFinder.Name(emojiChar).First()
	ui := emojipicker.NewUICtx(tconn)

	return uiauto.Combine("input emoji with emoji picker",
		// Launch the emoji keyboard.
		kb.AccelAction("Search+Shift+Space"),
		emojipicker.WaitUntilExists(tconn),
		// Select the emoji.
		ui.LeftClick(emojiCharFinder),
		// The emoji picker should disappear after we click on an emoji.
		emojipicker.WaitUntilGone(tconn),
	)
}
