// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package googledocs provides functions to interact with
// Google Docs.
package googledocs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
)

// FontStyle represents Bold, Italic, and Underline font styles.
type FontStyle = string

const (
	// Bold represents bolded font.
	Bold FontStyle = "Bold"

	// Italic represents italicized font.
	Italic FontStyle = "Italic"

	// Underline represents underlined font.
	Underline FontStyle = "Underline"
)

// UpdateTitle replaces the active Google Doc title with |title|.
func UpdateTitle(ctx context.Context, pc pointer.Context, ac *uiauto.Context, kw *input.KeyboardEventWriter, title string) error {
	titleField := nodewith.HasClass("docs-title-input").Role(role.TextField)
	return uiauto.Combine(
		"update the Google Doc title",
		pc.Click(titleField),
		ac.WaitUntilNoEvent(titleField, event.LocationChanged),
		kw.AccelAction("Backspace"),
		ac.WaitUntilNoEvent(titleField, event.LocationChanged),
		kw.TypeAction(title),
	)(ctx)
}

// EnablePageNumbers enables page numbers for the active Google Doc.
func EnablePageNumbers(ctx context.Context, pc pointer.Context, ac *uiauto.Context) error {
	format := nodewith.Name("Format").Role(role.MenuItem)
	pageNumbers := nodewith.NameStartingWith("Page numbers").Role(role.MenuItem)
	apply := nodewith.Name("Apply").Role(role.Button)
	return uiauto.Combine(
		"enable page numbers in Google Docs",
		ac.RetryUntil(
			pc.Click(format),
			ac.WithTimeout(5*time.Second).WaitUntilExists(pageNumbers),
		),
		pc.Click(pageNumbers),
		ac.WaitUntilExists(apply),
		pc.Click(apply),
		ac.WaitUntilGone(apply),
	)(ctx)
}

// UpdateTextStyleAction toggles the |style| button.
func UpdateTextStyleAction(ctx context.Context, pc pointer.Context, ac *uiauto.Context, style FontStyle) action.Action {
	toggle := nodewith.NameStartingWith(style).Role(role.ToggleButton)
	return uiauto.Combine(
		fmt.Sprintf("update text style to %s", style),
		pc.Click(toggle),
		ac.WaitUntilNoEvent(toggle, event.LocationChanged),
	)
}

// UpdatePagelessModeAction enables or disables pageless mode based
// on |isEnabled|. |docsConn| is the connection to the Google Doc tab,
// and is used to automatedly click the "OK" button through Javascript.
func UpdatePagelessModeAction(pc pointer.Context, docsConn *chrome.Conn, ac *uiauto.Context, kw *input.KeyboardEventWriter, isEnabled bool) action.Action {
	toggle := "Pages"
	if isEnabled {
		toggle = "Pageless"
	}

	file := nodewith.Name("File").Role(role.MenuItem)
	menu := nodewith.Role(role.Menu).Ancestor(docsWebArea)
	pageSetup := nodewith.NameStartingWith("Page setup").Role(role.MenuItem)
	mode := nodewith.NameStartingWith(toggle).Role(role.Tab)
	pageSetupDialog := nodewith.NameStartingWith("Page setup").Role(role.Dialog)
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(pageSetupDialog)
	return uiauto.Combine(
		fmt.Sprintf("set pageless mode to %t", isEnabled),
		pc.Click(file),
		// In chromevox subcase pageSetup would be hidden,
		// so we need to add keyboard actions to make it show up.
		ac.WaitForLocation(menu),
		uiauto.IfFailThen(
			ac.Exists(pageSetup.Onscreen()),
			ac.RetryUntil(
				kw.AccelAction("Up"),
				ac.WithTimeout(5*time.Second).WaitUntilExists(pageSetup),
			),
		),
		pc.Click(pageSetup),
		ac.WaitUntilExists(mode),
		pc.Click(mode),
		ac.WaitUntilExists(okButton),
		// TODO(b/232554445): An existing bug with opening pageless mode
		// prevents the "OK" button from appearing on screen, since the
		// currently visible modal is wrongly centered. A crude
		// workaround is to find the HTML element and trigger the
		// click itself through Javascript. This also addresses a
		// separate issue, where Lacros does not accurately calculate
		// the location of the "OK" button.
		ac.DoDefault(okButton),
		ac.WaitUntilGone(okButton),
	)
}
