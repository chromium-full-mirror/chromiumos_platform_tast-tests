// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shortcutcustomization contains drivers for controlling the ui of shortcut customization SWA.
package shortcutcustomization

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Launch starts the Shortcut Customization app via ctrl+alt+/.
func Launch(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Launch Shortcut customization app with ctrl+alt+/.
		if err := kb.Accel(ctx, "Ctrl+Alt+/"); err != nil {
			return errors.Wrap(err, "failed pressing ctrl+alt+/")
		}
		// Verify Shortcut customization app is launched.
		if err = ash.WaitForApp(ctx, tconn, apps.ShortcutCustomization.ID, 20*time.Second); err != nil {
			return errors.Wrap(err, "could not find app in shelf after launch")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed launching Shortcut customization app")
	}

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	shortcutCustomizationRootNode := nodewith.Name(apps.ShortcutCustomization.Name).Role(role.Window)
	if err := ui.WaitUntilExists(shortcutCustomizationRootNode)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find shortcut customization app")
	}

	return shortcutCustomizationRootNode, nil
}

// VerifyShortcutCustomizationIsLaunched checks that the app is open and all essential elements are in place.
func VerifyShortcutCustomizationIsLaunched(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	if err := ash.WaitForApp(ctx, tconn, apps.ShortcutCustomization.ID, time.Minute); err != nil {
		return errors.Wrap(err, "could not find app in shelf after launch")
	}
	// Verify categories exist.
	for _, category := range []string{"General", "Device", "Browser", "Text", "Windows and Desks", "Accessibility"} {
		if err := uiauto.Combine(fmt.Sprintf("Verify %q category exists", category),
			ui.WaitUntilExists(nodewith.Name(category).Role(role.Button)),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find %q category", category)
		}
	}
	// Verify essential elements exist.
	keyboardsettinglink := nodewith.Name("Keyboard settings").Role(role.Link)
	searchBar := nodewith.Name("Search shortcuts").Role(role.SearchBox)
	if err := uiauto.Combine("Verify essential elements exist",
		ui.WaitUntilExists(keyboardsettinglink),
		ui.WaitUntilExists(searchBar),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find element")
	}

	return nil
}

// VerifySubcategory checks that the subcategories in each category exists.
func VerifySubcategory(ctx context.Context, ui *uiauto.Context, subcategories []string) error {
	for _, subcategory := range subcategories {
		if err := uiauto.Combine(fmt.Sprintf("Verify %q subcategories exists", subcategory),
			ui.WaitUntilExists(nodewith.Name(subcategory).Role(role.StaticText)),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find %q subcategory", subcategory)
		}
	}

	return nil
}
