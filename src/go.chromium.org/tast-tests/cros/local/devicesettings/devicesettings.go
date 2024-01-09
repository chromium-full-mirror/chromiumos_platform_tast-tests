// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package devicesettings contains utilities for working with the device settings
// page.
package devicesettings

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
)

// Remap changes the action performed by a modifier key.
func Remap(ctx context.Context, ui *uiauto.Context, from, to string) error {
	keyRow := nodewith.Name(from).Role(role.GenericContainer)
	key := nodewith.Name(from).Role(
		role.ComboBoxSelect).Ancestor(keyRow)
	option := nodewith.Name(to).Role(role.ListBoxOption)
	if err := uiauto.Combine(fmt.Sprintf("choose %q option", to),
		ui.LeftClickUntil(key, ui.WithTimeout(
			2*time.Second).WaitUntilExists(option)),
		ui.LeftClick(option),
		ui.WaitUntilExists(option),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to choose %q option", to)
	}
	return nil
}

// NavigateMouseCustomization navigates control to mouse button customzation page
func NavigateMouseCustomization(ctx context.Context, ui *uiauto.Context, mouseName string) error {

	if err := ui.DoDefault(constants.MouseRow)(ctx); err != nil {
		return errors.Wrap(err, "failed to click mouse row")
	}

	mouseHeading := nodewith.NameContaining(mouseName).Role(role.Heading)
	if err := ui.WaitUntilExists(mouseHeading)(ctx); err != nil {
		return errors.Wrapf(err, "failed to find the connected mouse %s ", mouseName)
	}

	// Verify if the customization link shows up in the mouse section.
	customizeLink := nodewith.Name("Customize mouse buttons").Role(role.Link).First()
	if err := ui.WithTimeout(2 * time.Minute).LeftClick(customizeLink)(ctx); err != nil {
		return errors.Wrap(err, "failed to Find or click customize mouse buttons link Customize mouse buttons")
	}

	// Verify if the virtual mouse connected shows up.
	mouseText := nodewith.NameContaining("Click additional buttons on your " + mouseName).Role(role.StaticText)
	if err := ui.WaitUntilExists(mouseText)(ctx); err != nil {
		return errors.Wrapf(err, "failed to find text Tast virtual mouse in customization page %s", mouseName)
	}

	return nil
}

// ClearTextArea allows to clear the text area for the button name while on edit mode.
func ClearTextArea(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter, buttonNameText string) error {

	buttonNameTextbox := nodewith.Name(buttonNameText).Role(role.TextField)
	if err := uiauto.Combine("Clear the textbox for button name",
		ui.WaitUntilExists(buttonNameTextbox.Focused()),
		kb.AccelAction("Ctrl+A"),
		kb.TypeKeyAction(input.KEY_BACKSPACE),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to clear text at %s for the existing Other Button 1 name", buttonNameText)
	}

	return nil
}
