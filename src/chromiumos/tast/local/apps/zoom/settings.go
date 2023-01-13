// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"fmt"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

var (
	settingsDialog    = nodewith.Name("settings dialog window").Role(role.Application).Ancestor(zoomMainWebArea)
	moreOptionsButton = nodewith.Name("More meeting control").Ancestor(zoomMainWebArea)
	closeDialogButton = nodewith.Role(role.Button).HasClass("settings-dialog__close").Ancestor(settingsDialog)

	backgroundTab = nodewith.Name("Background").Role(role.Tab).Ancestor(settingsDialog)
)

// ChangeSettings changes one or more settings from main screen.
// It opens the settings page before and closes it after.
func (zm *Zoom) ChangeSettings(actions ...action.Action) action.Action {
	actionsToPerform := []action.Action{zm.openSettings}
	actionsToPerform = append(actionsToPerform, actions...)
	actionsToPerform = append(actionsToPerform, zm.closeSettings)
	return uiauto.NamedCombine("change settings",
		actionsToPerform...,
	)
}

// SetJoinAudio chooses an audio option or dismisses the dialog on the Join Audio page.
func (zm *Zoom) SetJoinAudio(expectedValue, isDialogAlreadyOpened bool) action.Action {
	joinAudioByComputerButton := nodewith.Name("Join Audio by Computer").Role(role.Button)
	closeButton := nodewith.Name("close").HasClass("join-dialog__close").Role(role.Button)
	joinAudioButton := nodewith.Name("Join Audio").Role(role.Button).Focusable()
	ui := zm.ui

	return func(ctx context.Context) error {
		if !expectedValue && !isDialogAlreadyOpened {
			return nil
		}

		if isDialogAlreadyOpened {
			if err := ui.WaitUntilExists(closeButton)(ctx); err != nil {
				return errors.Wrap(err, "join audio dialog is not found")
			}
			if !expectedValue {
				return ui.DoDefaultUntil(
					closeButton,
					ui.WithTimeout(shortUITimeout).WaitUntilGone(closeButton))(ctx)
			}
		}

		return uiauto.Combine("join audio by computer",
			ui.DoDefault(joinAudioButton),
			ui.DoDefault(joinAudioByComputerButton),
			ui.WaitUntilGone(closeButton),
		)(ctx)
	}
}

// SwitchVideo switches on / off camera.
// It turns on video if value is true, otherwise turns off video.
// It skips action if the camera status is already as expected.
func (zm *Zoom) SwitchVideo(value bool) action.Action {
	ui := zm.ui

	return func(ctx context.Context) error {
		cameraToggleButton, err := ui.FindAnyExists(ctx, startVideoButton, stopVideoButton)
		if err != nil {
			return errors.Wrap(err, "failed to find video toggle button")
		}
		if value && cameraToggleButton == startVideoButton {
			return ui.DoDefaultUntil(startVideoButton,
				ui.WithTimeout(shortUITimeout).WaitUntilExists(stopVideoButton))(ctx)
		} else if !value && cameraToggleButton == stopVideoButton {
			return ui.DoDefaultUntil(stopVideoButton,
				ui.WithTimeout(shortUITimeout).WaitUntilExists(startVideoButton))(ctx)
		}
		return nil
	}
}

// SetBackgroundBlur chooses "Blur" option in "Background" Tab.
func (zm *Zoom) SetBackgroundBlur(ctx context.Context) error {
	return zm.chooseBackground("blur.jpg selected")(ctx)
}

// SetBackgroundNone chooses "None" option in "Background" Tab.
func (zm *Zoom) SetBackgroundNone(ctx context.Context) error {
	return zm.chooseBackground("None selected")(ctx)
}

// chooseBackground chooses background option in "Background" Tab.
func (zm *Zoom) chooseBackground(optionName string) action.Action {
	ui := zm.ui
	backgroundItem := nodewith.NameContaining(optionName).Role(role.ListBoxOption).Ancestor(settingsDialog)

	return uiauto.Combine(fmt.Sprintf("set background to %q", optionName),
		ui.LeftClickUntil(backgroundTab,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(backgroundTab.Focused())),
		ui.DoDefaultUntil(backgroundItem,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(backgroundItem.Focused())),
		// After applying the new background, give it 3 seconds to load the new background.
		// TODO(b/264370256): Work out a better way to check background effect rather than sleep.
		uiauto.Sleep(shortUITimeout),
	)
}

// openSettings opens settings page in Zoom.
func (zm *Zoom) openSettings(ctx context.Context) error {
	ui := zm.ui
	settingsButton := nodewith.Name("Settings").Role(role.Button).Ancestor(zoomMainWebArea)

	if err := ui.Exists(settingsDialog)(ctx); err == nil {
		testing.ContextLog(ctx, "Settings page is already opened")
		return nil
	}

	return uiauto.Combine("open Meet settings page",
		zm.showInterface,
		// If the screen width is not enough, the settings button will be moved to more options.
		// So checking whether if the settings button is on screen, otherwise clicks More button to expand menu.
		func(ctx context.Context) error {
			if settingsButtonFound, err := ui.IsNodeFound(ctx, settingsButton); err != nil {
				return err
			} else if settingsButtonFound {
				return nil
			}
			// Update settings finder to menu type.
			settingsButton = nodewith.Name("Settings").Role(role.MenuItem).Ancestor(zoomMainWebArea)
			return ui.DoDefaultUntil(moreOptionsButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(settingsButton))(ctx)
		},
		ui.LeftClickUntil(settingsButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(settingsDialog)),
	)(ctx)
}

// closeSettings closes the settings page in Zoom.
func (zm *Zoom) closeSettings(ctx context.Context) error {
	return zm.ui.DoDefaultUntil(
		closeDialogButton,
		zm.ui.WithTimeout(shortUITimeout).WaitUntilGone(settingsDialog),
	)(ctx)
}
