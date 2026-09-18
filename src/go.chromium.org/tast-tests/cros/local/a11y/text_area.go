// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// TextArea represents a browser tab containing a focused textarea for input testing.
type TextArea struct {
	conn *chrome.Conn
	ui   *uiauto.Context
	node *nodewith.Finder
}

// OpenTextAreaTab opens a new browser tab with a focused textarea element.
func OpenTextAreaTab(ctx context.Context, cr *chrome.Chrome, ui *uiauto.Context) (*TextArea, error) {
	textURL := URLFromHTML(`<textarea id="textInput" rows="10" cols="80"></textarea>`)
	conn, err := NewTabWithURL(ctx, cr, textURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open textarea URL")
	}
	node := nodewith.Role(role.TextField).Ancestor(nodewith.HasClass("ContentsWebView"))
	ta := &TextArea{conn: conn, ui: ui, node: node}
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(node)(ctx); err != nil {
		ta.Close(ctx)
		return nil, errors.Wrap(err, "text field node did not appear")
	}
	if err := ui.WithTimeout(5 * time.Second).LeftClickUntilFocused(node)(ctx); err != nil {
		ta.Close(ctx)
		return nil, errors.Wrap(err, "failed to click and focus text field")
	}
	if err := ta.Focus(ctx); err != nil {
		ta.Close(ctx)
		return nil, errors.Wrap(err, "failed to focus text field")
	}
	return ta, nil
}

// Close closes the underlying tab target and Chrome connection.
func (ta *TextArea) Close(ctx context.Context) {
	ta.conn.CloseTarget(ctx)
	ta.conn.Close()
}

// Focus ensures the textarea element is focused in both the DOM and accessibility tree.
func (ta *TextArea) Focus(ctx context.Context) error {
	if err := ta.conn.Eval(ctx, `(() => {
		const el = document.getElementById('textInput');
		if (!el) throw new Error('textarea #textInput not found');
		if (document.activeElement !== el) el.focus();
	})()`, nil); err != nil {
		return err
	}
	return ta.ui.WithTimeout(5 * time.Second).WaitUntilExists(ta.node.Focused())(ctx)
}

// Clear clears the textarea via DOM evaluation and waits for its StaticText node to disappear.
func (ta *TextArea) Clear(ctx context.Context) error {
	if err := ta.conn.Eval(ctx, `(() => {
		const el = document.getElementById('textInput');
		if (el) {
			el.value = '';
			el.dispatchEvent(new Event('input', { bubbles: true }));
		}
	})()`, nil); err != nil {
		return errors.Wrap(err, "failed to clear textarea via DOM")
	}
	textNode := nodewith.Role(role.StaticText).Ancestor(ta.node)
	return ta.ui.WithTimeout(5 * time.Second).WaitUntilGone(textNode)(ctx)
}

// WaitForText waits for the textarea's StaticText node to match expected (or disappear if
// expected is empty), wrapping any failure with the actual DOM value.
func (ta *TextArea) WaitForText(ctx context.Context, expected string) error {
	textNode := nodewith.Role(role.StaticText).Ancestor(ta.node)
	var err error
	if expected != "" {
		err = ta.ui.WithTimeout(5 * time.Second).WaitUntilExists(textNode.Name(expected))(ctx)
	} else {
		err = ta.ui.WithTimeout(5 * time.Second).WaitUntilGone(textNode)(ctx)
	}
	if err != nil {
		return ta.wrapErrWithValue(ctx, err)
	}
	return nil
}

// WaitForNonEmptyText waits for any StaticText node to appear inside the textarea,
// wrapping any failure with the actual DOM value.
func (ta *TextArea) WaitForNonEmptyText(ctx context.Context) error {
	textNode := nodewith.Role(role.StaticText).Ancestor(ta.node).First()
	if err := ta.ui.WithTimeout(5 * time.Second).WaitUntilExists(textNode)(ctx); err != nil {
		return ta.wrapErrWithValue(ctx, err)
	}
	return nil
}

func (ta *TextArea) wrapErrWithValue(ctx context.Context, err error) error {
	var actualVal string
	if evalErr := ta.conn.Eval(ctx, `document.getElementById('textInput')?.value || ''`, &actualVal); evalErr == nil {
		return errors.Wrapf(err, "DOM textarea value is %q", actualVal)
	}
	return err
}
