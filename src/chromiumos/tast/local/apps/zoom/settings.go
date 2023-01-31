// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/testing"
)

var (
	settingsDialog    = nodewith.Name("settings dialog window").Role(role.Application).Ancestor(zoomMainWebArea)
	moreOptionsButton = nodewith.Name("More meeting control").Ancestor(zoomMainWebArea)
	closeDialogButton = nodewith.Role(role.Button).HasClass("settings-dialog__close").Ancestor(settingsDialog)

	backgroundTab   = nodewith.Name("Background").Role(role.Tab).Ancestor(settingsDialog)
	backgroundPanel = nodewith.Name("Background").Role(role.TabPanel).Ancestor(settingsDialog)
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

// expandAudioOption expands audio option menu.
func (zm *Zoom) expandAudioOption(ctx context.Context) error {
	ui := zm.ui
	moreAudioControlsButton := nodewith.Name("More audio controls").Role(role.Button)
	moreAudioControlsMenu := nodewith.Name("More audio controls").Role(role.Menu)
	return uiauto.NamedCombine("expand audio option",
		ui.LeftClickUntil(moreAudioControlsButton,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(moreAudioControlsMenu)))(ctx)
}

// leaveComputerAudio leaves computer audio from audio option menu.
func (zm *Zoom) leaveComputerAudio(ctx context.Context) error {
	ui := zm.ui
	leaveComputerAudioItem := nodewith.Name("Leave Computer Audio").Role(role.MenuItem)
	return uiauto.NamedCombine("leave computer audio",
		zm.expandAudioOption,
		ui.LeftClickUntil(leaveComputerAudioItem,
			ui.WithTimeout(shortUITimeout).WaitUntilGone(leaveComputerAudioItem)))(ctx)
}

// SetJoinAudio chooses an audio option or dismisses the dialog on the Join Audio page.
func (zm *Zoom) SetJoinAudio(expectedValue bool) action.Action {
	ui := zm.ui
	joinAudioByComputerButton := nodewith.Name("Join Audio by Computer").Role(role.Button)
	closeButton := nodewith.Name("close").HasClass("join-dialog__close").Role(role.Button)
	joinAudioButton := nodewith.Name("join audio").Role(role.Button).Focusable()
	dismissJoinAudioDialog := ui.DoDefaultUntil(
		closeButton,
		ui.WithTimeout(shortUITimeout).WaitUntilGone(closeButton))
	triggerJoinAudioDialog := ui.DoDefaultUntil(
		joinAudioButton,
		ui.WithTimeout(shortUITimeout).WaitUntilExists(joinAudioByComputerButton))

	return func(ctx context.Context) error {
		audioButton, err := ui.FindAnyExists(ctx, unmuteButton, muteButton, joinAudioButton)
		if err != nil {
			return errors.Wrap(err, "failed to find audio buttons")
		}

		if audioButton != joinAudioButton {
			if expectedValue {
				testing.ContextLog(ctx, "It has automatically joined audio")
				return nil
			}
			// Leave audio since it it not expected.
			return uiauto.Combine("leave audio",
				zm.leaveComputerAudio,
				dismissJoinAudioDialog,
			)(ctx)
		}

		// Check whether `Join audio dialog` is automatically shown up.
		// This can add 10s wait time if user decides to join audio after initial setup. But it is unusual.
		if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(joinAudioByComputerButton)(ctx); err != nil {
			if !expectedValue {
				testing.ContextLog(ctx, "Audio is not joined")
				return nil
			}
			if err := triggerJoinAudioDialog(ctx); err != nil {
				return err
			}
		}

		// Join audio dialog is shown but expected not to join audio.
		if !expectedValue {
			return dismissJoinAudioDialog(ctx)
		}

		return ui.DoDefaultUntil(
			joinAudioByComputerButton,
			ui.WithTimeout(shortUITimeout).WaitUntilGone(joinAudioByComputerButton),
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
			return uiauto.RetrySilently(5, uiauto.Combine("turn on the camera",
				prompts.ActionAndGrantPermissionIfRequired(
					zm.tconn, zm.conn, zm.ui.DoDefault(cameraToggleButton), webutil.PermissionCamera),
				ui.WithTimeout(5*time.Second).WaitUntilGone(cameraToggleButton)),
			)(ctx)
		} else if !value && cameraToggleButton == stopVideoButton {
			return uiauto.NamedCombine("turn off the camera",
				ui.WithTimeout(mediumUITimeout).DoDefaultUntil(stopVideoButton,
					ui.WaitUntilGone(stopVideoButton)),
				ui.WaitUntilExists(startVideoButton))(ctx)
		}
		return nil
	}
}

// SwitchAudio switches on / off microphone.
// It turns on audio if value is true, otherwise turns off audio.
// It skips action if the microphone status is already as expected.
func (zm *Zoom) SwitchAudio(value bool) action.Action {
	ui := zm.ui

	return func(ctx context.Context) error {
		audioToggleButton, err := ui.FindAnyExists(ctx, unmuteButton, muteButton)
		if err != nil {
			return errors.Wrap(err, "failed to find audio toggle button")
		}
		if value && audioToggleButton == unmuteButton {
			return uiauto.NamedCombine("turn on the microphone",
				ui.WithTimeout(mediumUITimeout).DoDefaultUntil(unmuteButton,
					ui.WaitUntilGone(unmuteButton)),
				ui.WaitUntilExists(muteButton))(ctx)
		} else if !value && audioToggleButton == muteButton {
			return uiauto.NamedCombine("turn off the microphone",
				ui.WithTimeout(mediumUITimeout).DoDefaultUntil(muteButton,
					ui.WaitUntilGone(muteButton)),
				ui.WaitUntilExists(unmuteButton))(ctx)
		}
		return nil
	}
}

// BackgroundOption indicates the option name of the background that can be selected.
type BackgroundOption string

const (
	// NoBackground is the no background option.
	NoBackground BackgroundOption = "None"
	// BlurBackground is the blur background option.
	BlurBackground BackgroundOption = "blur.jpg"
	// StaticBackground is the static background option.
	StaticBackground BackgroundOption = "San Francisco.jpg"
)

// SetBackgroundBlur chooses "Blur" option in "Background" Tab.
func (zm *Zoom) SetBackgroundBlur(ctx context.Context) error {
	return zm.ChooseBackground(BlurBackground)(ctx)
}

// SetBackgroundNone chooses "None" option in "Background" Tab.
func (zm *Zoom) SetBackgroundNone(ctx context.Context) error {
	return zm.ChooseBackground(NoBackground)(ctx)
}

// ChooseBackground chooses background option in "Background" Tab.
func (zm *Zoom) ChooseBackground(optionName BackgroundOption) action.Action {
	ui := zm.ui
	backgroundName := string(optionName) + " selected"
	backgroundOption := nodewith.NameContaining(backgroundName).Role(role.ListBoxOption).Ancestor(settingsDialog)

	return uiauto.NamedCombine(fmt.Sprintf("set background to %q", optionName),
		ui.LeftClickUntil(backgroundTab,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(backgroundPanel)),
		ui.DoDefaultUntil(backgroundOption,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(backgroundOption.Focused())),
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

	// Sometimes the menu will disappear and need to retry to open.
	return uiauto.Retry(3, uiauto.Combine("open Meet settings page",
		zm.ShowInterface,
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
		ui.WithTimeout(longUITimeout).LeftClickUntil(settingsButton, ui.WaitUntilExists(settingsDialog)),
	))(ctx)
}

// closeSettings closes the settings page in Zoom.
func (zm *Zoom) closeSettings(ctx context.Context) error {
	return zm.ui.DoDefaultUntil(
		closeDialogButton,
		zm.ui.WithTimeout(shortUITimeout).WaitUntilGone(settingsDialog),
	)(ctx)
}
