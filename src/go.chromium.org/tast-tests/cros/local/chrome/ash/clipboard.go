// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ash

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ClipboardTextData returns clipboard text data.
func ClipboardTextData(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	var data string

	if err := tconn.Call(ctx, &data, `tast.promisify(chrome.autotestPrivate.getClipboardTextData)`); err != nil {
		return "", err
	}

	return data, nil
}

// SetClipboard forcibly sets the clipboard to the given data.
func SetClipboard(ctx context.Context, tconn *chrome.TestConn, data string) error {
	return tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.setClipboardTextData)`, data)
}

// WaitUntilClipboardText waits for the clipboard text to equal the given data with a 5 second timeout.
// This function should be used after ctrl+C to check for copied text since ClipboardTextData can run faster than ctrl+C.
func WaitUntilClipboardText(ctx context.Context, tconn *chrome.TestConn, data string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		clipData, err := ClipboardTextData(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get clipboard content")
		}
		if clipData != data {
			return errors.Errorf("clipboard data mismatch: got %q, want %q", clipData, data)
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 5 * time.Second,
	}); err != nil {
		return err
	}

	return nil
}
