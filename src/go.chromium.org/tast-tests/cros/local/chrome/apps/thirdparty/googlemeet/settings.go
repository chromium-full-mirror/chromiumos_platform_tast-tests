// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	settingsDialog           = nodewith.Name("Settings").Role(role.Dialog).Ancestor(meetRootWebArea)
	changeLayoutDialog       = nodewith.Name("Change layout").Role(role.Dialog).Ancestor(meetRootWebArea)
	videoSettingsTabButton   = nodewith.Name("Video").Role(role.Tab).Ancestor(settingsDialog)
	generalSettingsTabButton = nodewith.Name("General").Role(role.Tab).Ancestor(settingsDialog)
	closeSettingsButton      = nodewith.Name("Close dialog").Role(role.Button).Ancestor(settingsDialog)
	resolutionButtonReg      = regexp.MustCompile("Auto|High definition|Standard definition")
	resolutionButton         = nodewith.NameRegex(resolutionButtonReg).Role(role.ComboBoxMenuButton).Ancestor(settingsDialog)
)

// OpenSettings opens settings page in GoogleMeet.
func (gm *GoogleMeet) OpenSettings(ctx context.Context) error {
	settingsButton := nodewith.Name("Settings").Role(role.MenuItem).Ancestor(meetRootWebArea)

	if err := gm.ui.Exists(settingsDialog)(ctx); err == nil {
		testing.ContextLog(ctx, "Settings page is already opened")
		return nil
	}

	return uiauto.Combine("open Meet settings page",
		gm.doDefaultMoreOptions,
		gm.ui.DoDefaultUntil(settingsButton, gm.ui.WithTimeout(shortUITimeout).WaitUntilExists(settingsDialog)),
	)(ctx)
}

// CloseSettings closes the settings page in GoogleMeet.
func (gm *GoogleMeet) CloseSettings(ctx context.Context) error {
	return gm.ui.DoDefaultUntil(
		closeSettingsButton,
		gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(settingsDialog),
	)(ctx)
}

// SetLeaveEmptyCalls sets the option of "Leave empty calls" in "General" Tab.
func (gm *GoogleMeet) SetLeaveEmptyCalls(value bool) action.Action {
	finder := nodewith.Name("Leave empty calls").Role(role.Switch).Ancestor(settingsDialog)
	actionDesc := `set "leave empty calls" to false`
	if value {
		actionDesc = `set "leave empty calls" to true`
	}

	return uiauto.NamedCombine(actionDesc,
		gm.ui.DoDefault(generalSettingsTabButton),
		gm.setToggleValue(finder, value),
	)
}

// SetAdjustVideoLighting sets the option of "Adjust video lighting" in "Video"
// Tab. Requires that the video effects pane is open.
func (gm *GoogleMeet) SetAdjustVideoLighting(value bool) action.Action {
	appearanceTabFinder := nodewith.Name("Appearance").Role(role.Tab)
	backgroundsTabFinder := nodewith.Name("Backgrounds").Role(role.Tab)
	finder := nodewith.Name("Adjust video lighting").Role(role.Switch)

	return uiauto.Combine(
		fmt.Sprintf("set \"Adjust video lighting\" to %t", value),
		gm.ui.DoDefault(appearanceTabFinder),
		gm.setToggleValue(finder, value),
		gm.ui.DoDefault(backgroundsTabFinder),
	)
}

// ResolutionOption represents the resolution options can be set in Google Meet.
type ResolutionOption string

// Available options of resolutions in Google Meet.
const (
	ResolutionAuto        ResolutionOption = "Auto"
	ResolutionFullHD1080P                  = "Full high definition (1080p)"
	ResolutionHD720P                       = "High definition (720p)"
	ResolutionSD360P                       = "Standard definition (360p)"
	ResolutionLD180P                       = "Low definition (180p)"
)

// SetSendResolution sets the option of "Send Resolution" in "Video" Tab.
func (gm *GoogleMeet) SetSendResolution(value ResolutionOption) action.Action {
	sendResolutionButton := resolutionButton.First()
	sendResolutionOption := nodewith.Name(string(value)).Role(role.ListBoxOption).Ancestor(settingsDialog)
	actionDesc := fmt.Sprintf(`set "Send Resolution" to %q`, value)

	return uiauto.Combine(actionDesc,
		gm.ui.DoDefault(videoSettingsTabButton),
		gm.setDropdownValue(sendResolutionButton, sendResolutionOption, string(value)),
	)
}

// SetReceiveResolution sets the option of "Receive Resolution" in "Video" Tab.
func (gm *GoogleMeet) SetReceiveResolution(value ResolutionOption) action.Action {
	receiveResolutionButton := resolutionButton.Nth(1)
	receiveResolutionOption := nodewith.Name(string(value)).Role(role.ListBoxOption).Ancestor(settingsDialog)

	return uiauto.Combine(fmt.Sprintf(`set "Receive Resolution" to %q`, value),
		gm.ui.DoDefault(videoSettingsTabButton),
		gm.setDropdownValue(receiveResolutionButton, receiveResolutionOption, string(value)),
	)
}

func (gm *GoogleMeet) setToggleValue(finder *nodewith.Finder, value bool) action.Action {
	return func(ctx context.Context) error {
		shouldBeChecked := checked.False
		if value {
			shouldBeChecked = checked.True
		}

		info, err := gm.ui.Info(ctx, finder)
		if err != nil {
			return errors.Wrapf(err, "failed to get node info of %v", finder)
		}
		// Do nothing if the toggle value is already as expected.
		if info.Checked != shouldBeChecked {
			return gm.ui.DoDefaultUntil(
				finder,
				gm.ui.WithTimeout(shortUITimeout).WaitUntilCheckedState(finder, value),
			)(ctx)
		}
		return nil
	}
}

func (gm *GoogleMeet) setDropdownValue(dropdown, option *nodewith.Finder, value string) action.Action {
	return func(ctx context.Context) error {
		info, err := gm.ui.Info(ctx, dropdown)
		if err != nil {
			return errors.Wrapf(err, "failed to get node info of %v", dropdown)
		}
		if info.Name != value {
			return uiauto.Combine("select dropdown option",
				gm.ui.LeftClick(dropdown),
				gm.ui.LeftClick(option))(ctx)
		}
		return nil
	}
}

// EffectOption indicates the option name of the effect that can be selected.
type EffectOption string

const (
	// NoEffect is the no effect option.
	NoEffect EffectOption = "Turn off visual effects"
	// BlurEffect is the blur effect option.
	BlurEffect EffectOption = "Blur your background"
	// StaticEffect is the static effect option.
	StaticEffect EffectOption = "Filter showing you"
	// DynamicEffect is the dynamic effect option.
	DynamicEffect EffectOption = "Immersive background"
)

var effectsText = nodewith.Name("Effects").Role(role.StaticText).Ancestor(meetRootWebArea)

// OpenVideoEffects opens video effects page in GoogleMeet.
func (gm *GoogleMeet) OpenVideoEffects(ctx context.Context) error {
	applyVisualEffectsButton := nodewith.Name("Apply visual effects").Role(role.MenuItem).Ancestor(meetRootWebArea)

	return uiauto.Combine("open video effects setting dialog",
		gm.ui.WithTimeout(mediumUITimeout).RetryUntil(gm.doDefaultMoreOptions,
			gm.ui.WaitUntilExists(applyVisualEffectsButton)),
		gm.ui.WithTimeout(mediumUITimeout).DoDefaultUntil(applyVisualEffectsButton,
			gm.ui.WaitUntilExists(effectsText)),
	)(ctx)
}

// CloseVideoEffects closes the video effects page.
func (gm *GoogleMeet) CloseVideoEffects(ctx context.Context) error {
	closeApplyVisualEffectsButton := nodewith.Name("Close").Role(role.Button).Ancestor(meetRootWebArea).First()

	return uiauto.Combine("close video effects setting dialog",
		gm.ui.DoDefaultUntil(
			closeApplyVisualEffectsButton,
			gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(effectsText),
		),
	)(ctx)
}

// SetEffect sets the option of video effects.
func (gm *GoogleMeet) SetEffect(effectOption EffectOption) action.Action {
	// Effect option may conform to multiple effects. So add First() here.
	effectButton := nodewith.NameContaining(string(effectOption)).Role(role.ToggleButton).Ancestor(meetRootWebArea).First()

	return uiauto.NamedAction(fmt.Sprintf("set effect %q", effectOption),
		gm.setToggleValue(effectButton, true))
}

// SetBackgroundBlur enables/disables the background blur effect. Requires that
// the video effects pane is open.
func (gm *GoogleMeet) SetBackgroundBlur(value bool) action.Action {
	finder := nodewith.Name("Blur your background").Role(role.ToggleButton)
	backgroundsTabFinder := nodewith.Name("Backgrounds").Role(role.Tab)
	return uiauto.Combine(
		fmt.Sprintf("set \"Blur your background\" to %t", value),
		gm.ui.DoDefault(backgroundsTabFinder),
		gm.setToggleValue(finder, value),
	)
}

// SetEffectOnJoinPage sets the option of video effects on the join page.
// The "Get ready" dialog is the setup before joining a meeting room.
// Including changing visual effects.
func (gm *GoogleMeet) SetEffectOnJoinPage(effectOption EffectOption) action.Action {
	ui := gm.ui
	applyVisualEffectsButton := nodewith.Name("Apply visual effects").Role(role.Button).Ancestor(meetRootWebArea)
	getReadyDialog := nodewith.Name("Get ready").Role(role.Dialog).Ancestor(meetRootWebArea)
	effectsTab := nodewith.NameContaining("Effects").Role(role.Tab).Ancestor(getReadyDialog)
	effectButton := nodewith.Name(string(effectOption)).Role(role.ToggleButton).Ancestor(getReadyDialog)
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(getReadyDialog)
	selectAFileDialog := nodewith.Name("Select a file to open").ClassName("ExtensionViewViews")
	closeDialogButton := nodewith.Name("Close").Role(role.Button).Ancestor(selectAFileDialog)

	return uiauto.NamedCombine(fmt.Sprintf("set effect %q on the join page", effectOption),
		ui.WithTimeout(longUITimeout).DoDefaultUntil(applyVisualEffectsButton, ui.WaitUntilExists(effectsTab)),
		ui.LeftClick(effectsTab),
		ui.WithTimeout(mediumUITimeout).DoDefaultUntil(effectButton,
			ui.WaitUntilExists(effectButton.Focused())),
		ui.LeftClick(closeButton),
		// Some DUT performance is too poor, clicking the turn off button will trigger "Upload a background image".
		// If the dialog "select a file to open" is opened, close it.
		uiauto.IfSuccessThen(ui.WithTimeout(shortUITimeout).WaitUntilExists(selectAFileDialog),
			ui.LeftClick(closeDialogButton)))
}

// OpenChangeLayout opens change layout page in GoogleMeet.
func (gm *GoogleMeet) OpenChangeLayout(ctx context.Context) error {
	ui := gm.ui
	changeLayoutItem := nodewith.Name("Change layout").Role(role.MenuItem).Ancestor(meetRootWebArea)

	if err := ui.Exists(changeLayoutDialog)(ctx); err == nil {
		testing.ContextLog(ctx, "Change layout page is already opened")
		return nil
	}

	// It takes a long time to open the change layout panel when running classroom on low-end devices.
	// Therefore, here extend the timeout and increase the number of retry.
	return ui.Retry(5, uiauto.NamedCombine("open Meet layout page",
		uiauto.IfFailThen(ui.Exists(changeLayoutItem),
			ui.WithTimeout(longUITimeout).RetryUntil(gm.doDefaultMoreOptions, ui.WaitUntilExists(changeLayoutItem))),
		ui.DoDefault(changeLayoutItem),
		ui.WithTimeout(longUITimeout).WaitUntilExists(changeLayoutDialog)),
	)(ctx)
}

// doDefaultMoreOptions finds the correct 'More Option' button and clicks
// 'More Option' button via DoDefault.
func (gm *GoogleMeet) doDefaultMoreOptions(ctx context.Context) error {
	moreOptionsFinder := nodewith.Name("More options").Role(role.PopUpButton)
	moreOptionsButtons, err := gm.ui.NodesInfo(ctx, moreOptionsFinder)
	if err != nil || len(moreOptionsButtons) < 1 {
		return errors.Wrap(err, "failed to find more options button")
	}
	// Sometimes, the UI has two identical "More Options" buttons, which requires
	// selecting the last one to be the correct button.
	return gm.ui.DoDefault(moreOptionsFinder.Nth(len(moreOptionsButtons) - 1))(ctx)
}

// CloseChangeLayout closes the change layout page in GoogleMeet.
func (gm *GoogleMeet) CloseChangeLayout(ctx context.Context) error {
	ui := gm.ui
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(changeLayoutDialog)

	if err := ui.Gone(changeLayoutDialog)(ctx); err == nil {
		testing.ContextLog(ctx, "Change layout page is already closed")
		return nil
	}

	return uiauto.NamedAction("close change layout panel",
		ui.WithTimeout(longUITimeout).DoDefaultUntil(closeButton, ui.WaitUntilGone(closeButton)))(ctx)
}

// LayoutOption indicates the option name of the layout that can be selected.
type LayoutOption string

const (
	// TiledLayout is the "Tiled" layout option.
	TiledLayout = "Tiled"
	// SpotlightLayout is the "Spotlight" layout option.
	SpotlightLayout = "Spotlight"
)

// SetLayout sets the layout in GoogleMeet.
func (gm *GoogleMeet) SetLayout(layoutOption LayoutOption) action.Action {
	ui := gm.ui
	layoutRadioButton := nodewith.Name(string(layoutOption)).Role(role.RadioButton).Ancestor(changeLayoutDialog).Visible()

	return uiauto.NamedAction(fmt.Sprintf("set layout to %q", layoutOption),
		ui.DoDefaultUntil(layoutRadioButton,
			ui.WithTimeout(shortUITimeout).WaitUntilCheckedState(layoutRadioButton, true)))
}

// SetToMaxTiles sets the tile number to max in the Tiled layout.
// Return nil if it is not the Tiled layout.
func (gm *GoogleMeet) SetToMaxTiles(kb *input.KeyboardEventWriter, layoutOption LayoutOption) action.Action {
	return func(ctx context.Context) error {
		if layoutOption != TiledLayout {
			testing.ContextLogf(ctx, "Current layout %q is not the Tiled layout, cannot set tiles to the maximum", layoutOption)
			return nil
		}
		slider := nodewith.Name("Tiles").Role(role.Slider).First()
		raiseTiles := kb.AccelAction("Right")
		lowerTiles := kb.AccelAction("Left")
		isMaxTiles := func(ctx context.Context) error {
			// "49 tiles" is the maximum number of layout tiles.
			// It has nothing to do with the number of participants.
			const expectedResult = "49 tiles"
			sliderInfo, err := gm.ui.Info(ctx, slider)
			if err != nil {
				return errors.Wrap(err, "failed to get slider info")
			}
			value := sliderInfo.Value
			if value == expectedResult {
				return nil
			}
			return errors.Errorf("unexpected number of tiles: got %q, expected %q", value, expectedResult)
		}

		// Sometimes the DUT doesn't capture correct tile number.
		// Turn down the tiles number and try again.
		return gm.ui.Retry(3, uiauto.NamedCombine("set to max tiles",
			gm.ui.LeftClick(slider),
			uiauto.IfFailThen(
				// Keep raising tiles number until the max tiles number is reached.
				gm.ui.WithInterval(500*time.Millisecond).RetryUntil(raiseTiles, isMaxTiles),
				lowerTiles)))(ctx)
	}
}

// ShowInATile sets the layout shown in a tile when layout is "Tiled".
func (gm *GoogleMeet) ShowInATile() action.Action {
	ui := gm.ui
	showInATileButton := nodewith.Name("Show in a tile").Role(role.Button)

	return uiauto.NamedAction("show in a tile",
		uiauto.IfSuccessThen(ui.Exists(showInATileButton),
			ui.DoDefaultUntil(showInATileButton,
				ui.WithTimeout(shortUITimeout).WaitUntilGone(showInATileButton))))
}
