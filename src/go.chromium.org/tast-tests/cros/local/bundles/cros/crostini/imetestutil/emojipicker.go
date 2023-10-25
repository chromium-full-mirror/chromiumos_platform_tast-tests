// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imetestutil provides shared struct to test IME behaviour in Crostini.
package imetestutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime/emojipicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
)

// OpenEmojiPickerAndInputEmoji opens the emoji picker using the keyboard shortcut and selects the emojiChar.
func OpenEmojiPickerAndInputEmoji(ctx context.Context, cr *chrome.Chrome, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, emojiChar string) uiauto.Action {
	emojiCharFinder := emojipicker.NodeFinder.Name(emojiChar).First()
	ui := emojipicker.NewUICtx(tconn)

	return uiauto.Combine("input emoji with emoji picker",
		// Launch the emoji keyboard.
		kb.AccelAction("Search+Shift+Space"),
		emojipicker.WaitUntilExists(tconn),
		// With GIF picker flag on, there will be a nudge overlay when user opens emoji picker for the first time.
		// We dismiss it here by clicking the search field.
		ui.WaitUntilExists(emojipicker.SearchFieldFinder),
		ui.LeftClickUntil(emojipicker.SearchFieldFinder, ui.WithTimeout(30*time.Second).WaitUntilGone(emojipicker.NudgeOverlay)),
		// Select the emoji.
		ui.LeftClick(emojiCharFinder),
		// The emoji picker should disappear after we click on an emoji.
		emojipicker.WaitUntilGone(tconn),
	)
}
