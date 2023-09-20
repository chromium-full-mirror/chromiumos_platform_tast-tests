// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	settingsDialog    = nodewith.Name("settings dialog window").Role(role.Application).Ancestor(zoomMainWebArea)
	moreOptionsButton = nodewith.Name("More meeting control").Ancestor(zoomMainWebArea)
	closeDialogButton = nodewith.Role(role.Button).HasClass("settings-dialog__close").Ancestor(settingsDialog)

	backgroundTab   = nodewith.Name("Background").Role(role.Tab).Ancestor(settingsDialog)
	backgroundPanel = nodewith.Name("Background").Role(role.TabPanel).Ancestor(settingsDialog)

	infobarContainer = nodewith.Name("Infobar Container").Role(role.Group)
	// There may be multiple "Stop sharing" buttons, so add First() here.
	stopSharing = nodewith.Name("Stop sharing").Role(role.Button).Ancestor(infobarContainer).First()
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

var audioOptionsFinder = uidetection.TextBlockFromSentence("Audio Options")

// expandAudioOption expands audio option menu.
func (zm *Zoom) expandAudioOption(ctx context.Context) error {
	ui := zm.ui
	ud := uidetection.NewDefault(zm.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	moreAudioControlsButton := nodewith.Name("More audio controls").Role(role.Button)

	return uiauto.NamedAction("expand audio option",
		ui.DoDefaultUntil(
			moreAudioControlsButton,
			ud.WaitUntilExists(audioOptionsFinder),
		))(ctx)
}

// leaveComputerAudio leaves computer audio from audio option menu.
func (zm *Zoom) leaveComputerAudio(ctx context.Context) error {
	ud := uidetection.NewDefault(zm.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	leaveComputerAudioFinder := uidetection.TextBlockFromSentence("Leave Computer Audio").Above(audioOptionsFinder)
	return uiauto.NamedCombine("leave computer audio",
		zm.expandAudioOption,
		ud.LeftClick(leaveComputerAudioFinder),
		ud.WaitUntilGone(audioOptionsFinder),
	)(ctx)
}

// SetJoinAudio chooses an audio option or dismisses the dialog on the Join Audio page.
func (zm *Zoom) SetJoinAudio(expectedValue bool) action.Action {
	ui := zm.ui
	joinAudioByComputerButton := nodewith.Name("Join Audio by Computer").Role(role.Button).First()
	closeButton := nodewith.Name("close").HasClass("join-dialog__close").Role(role.Button)
	joinAudioButton := nodewith.Name("join audio").Role(role.Button).Focusable()
	dismissJoinAudioDialog := ui.DoDefaultUntil(
		closeButton,
		ui.WithTimeout(shortUITimeout).WaitUntilGone(closeButton))
	triggerJoinAudioDialog := uiauto.NamedAction("trigger 'Join Audio' dialog",
		ui.DoDefaultUntil(
			joinAudioButton,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(joinAudioByComputerButton)))

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

		return uiauto.RetrySilently(5, uiauto.Combine("join audio",
			prompts.ActionAndGrantPermissionIfRequired(
				zm.tconn, zm.conn, zm.ui.DoDefault(joinAudioByComputerButton), webutil.PermissionMicrophone),
			ui.WithTimeout(5*time.Second).WaitUntilGone(joinAudioByComputerButton)),
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

// LayoutMode indicates the mode name of the layout that can be selected.
type LayoutMode string

const (
	// SpeakerView is the speaker view layout mode.
	SpeakerView LayoutMode = "Speaker View"
	// GalleryView is the gallery view layout mode.
	GalleryView LayoutMode = "Gallery View"
)

// ChangeLayout changes the layout to a specific mode.
func (zm *Zoom) ChangeLayout(layoutMode LayoutMode) action.Action {
	return func(ctx context.Context) error {
		ui := zm.ui
		mode := string(layoutMode)
		viewButton := nodewith.Name("View").Role(role.Button)
		viewMenu := nodewith.Role(role.Menu).HasClass("dropdown-menu")
		modeNode := nodewith.Name(mode).Role(role.MenuItem)
		// Sometimes the zoom's menu disappears too fast, add retry to show view menu.
		showViewMenu := uiauto.Combine("show view menu",
			zm.ShowInterface,
			ui.LeftClickUntil(viewButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(viewMenu)))

		if err := uiauto.Combine("check view menu",
			showViewMenu,
			ui.WithTimeout(shortUITimeout).WaitUntilExists(modeNode),
		)(ctx); err != nil {
			// Some DUTs don't support layout changes. ('Speacker View' and 'Gallery View')
			testing.ContextLogf(ctx, "%q is not supported on this device, ignore changing the layout", mode)
			return nil
		}

		return ui.Retry(3, uiauto.NamedCombine("change layout to "+mode,
			uiauto.IfSuccessThen(ui.Gone(modeNode), showViewMenu),
			ui.LeftClick(modeNode)))(ctx)
	}
}

// TypingInChat opens chat panel and types message to chat box.
func (zm *Zoom) TypingInChat(kb *input.KeyboardEventWriter, message string) action.Action {
	ui := zm.ui
	chatButton := nodewith.Name("open the chat pane").Role(role.Button)
	chatTextRe := regexp.MustCompile("(Type message here ...|chat message)")
	chatTextField := nodewith.NameRegex(chatTextRe).Role(role.TextField)
	messageText := nodewith.Name(message).Role(role.StaticText).First()
	typeMessage := uiauto.NamedCombine("type message: "+message,
		ui.LeftClickUntil(chatTextField, ui.WithTimeout(shortUITimeout).WaitUntilExists(chatTextField.Focused())),
		kb.AccelAction("Ctrl+A"),
		kb.TypeAction(message),
		kb.AccelAction("enter"),
		ui.WaitUntilExists(messageText))

	return uiauto.NamedCombine("open chat window and type message",
		func(ctx context.Context) error {
			// Close all notifications to prevent them from covering the chat text field.
			return ash.CloseNotifications(ctx, zm.tconn)
		},
		uiauto.IfSuccessThen(ui.Gone(chatTextField), ui.DoDefault(chatButton)),
		ui.WaitUntilExists(chatTextField),
		ui.Retry(3, typeMessage),
	)
}

// CloseChatPanel closes chat panel.
func (zm *Zoom) CloseChatPanel() action.Action {
	ui := zm.ui
	manageChatPanel := nodewith.Name("Manage Chat Panel")
	manageChatPanelButton := manageChatPanel.Role(role.PopUpButton)
	manageChatPanelMenu := manageChatPanel.Role(role.Menu)
	closeButton := nodewith.Name("Close").Role(role.MenuItem).Ancestor(manageChatPanelMenu)

	return uiauto.NamedCombine("close chat panel",
		ui.LeftClick(manageChatPanelButton),
		ui.LeftClick(closeButton),
		ui.WaitUntilGone(manageChatPanelButton),
	)
}

// ShareScreen shares screen via "Chrome Tab" and select the tab to share.
func (zm *Zoom) ShareScreen(tabName string) action.Action {
	ui := zm.ui
	shareScreenButton := nodewith.Name("Share Screen").Role(role.Button)
	// There may be multiple "Chrome Tab" tabs, so add First() here.
	presentMode := nodewith.Name("Chrome Tab").Role(role.Tab).First()
	presentTab := nodewith.ClassName("AXVirtualView").Role(role.Cell).NameContaining(tabName)
	shareButton := nodewith.Name("Share").Role(role.Button)

	return uiauto.NamedCombine("share screen",
		zm.ShowInterface,
		ui.WithTimeout(longUITimeout).DoDefaultUntil(shareScreenButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(presentMode)),
		ui.LeftClick(presentMode),
		ui.LeftClick(presentTab),
		ui.LeftClickUntil(shareButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(shareButton)),
		ui.WithTimeout(mediumUITimeout).WaitUntilExists(stopSharing),
	)
}

// StopShareScreen stops sharing screen.
func (zm *Zoom) StopShareScreen() action.Action {
	ui := zm.ui
	return ui.LeftClickUntil(stopSharing, ui.WithTimeout(shortUITimeout).WaitUntilGone(stopSharing))
}

// closeSettings closes the settings page in Zoom.
func (zm *Zoom) closeSettings(ctx context.Context) error {
	return zm.ui.DoDefaultUntil(
		closeDialogButton,
		zm.ui.WithTimeout(shortUITimeout).WaitUntilGone(settingsDialog),
	)(ctx)
}
