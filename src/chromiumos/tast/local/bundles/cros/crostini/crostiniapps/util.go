// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crostiniapps contains shared code used for app tests.
package crostiniapps

import (
	"context"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"

	"go.chromium.org/tast/core/errors"
)

// CheckInputViaClipboard copies all text from the currently focused input text field and checks the clipboard content for a matching string.
// The input field must be focused before this function is called.
// Note that the cursor will be moved to the end of the input field before returning.
func CheckInputViaClipboard(ctx context.Context, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, expectedText string) error {
	if err := uiauto.Combine("copy text to clipboard",
		// Select all the text in the input box.
		keyboard.AccelAction("ctrl+A"),
		// Copy selected content.
		keyboard.AccelAction("ctrl+C"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy input to clipboard")
	}

	var clipData string
	if err := tconn.Eval(ctx, `tast.promisify(chrome.autotestPrivate.getClipboardTextData)()`, &clipData); err != nil {
		return errors.Wrap(err, "failed to get clipboard content")
	}
	if clipData != expectedText {
		return errors.Errorf("clipboard data mismatch: got %q, want %q", clipData, expectedText)
	}

	// Move cursor to the end of the text.
	if err := uiauto.Combine("deselect text",
		// Deselected content after checking.
		keyboard.AccelAction("Right"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to deselect text")
	}
	return nil
}
