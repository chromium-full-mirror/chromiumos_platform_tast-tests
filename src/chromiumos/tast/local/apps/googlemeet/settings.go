// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"fmt"
	"regexp"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

var (
	settingsDialog           = nodewith.Name("Settings").Role(role.Dialog).Ancestor(meetingWebview)
	videoSettingsTabButton   = nodewith.Name("Video").Role(role.Tab).Ancestor(settingsDialog)
	generalSettingsTabButton = nodewith.Name("General").Role(role.Tab).Ancestor(settingsDialog)
	closeSettingsButton      = nodewith.Name("Close dialog").Role(role.Button).Ancestor(settingsDialog)
)

// OpenSettings opens settings page in GoogleMeet.
func (gm *GoogleMeet) OpenSettings(ctx context.Context) error {
	settingsButton := nodewith.Name("Settings").Role(role.MenuItem).Ancestor(meetingWebview)

	if err := gm.ui.Exists(settingsDialog)(ctx); err == nil {
		testing.ContextLog(ctx, "Settings page is already opened")
		return nil
	}

	return uiauto.Combine("open Meet settings page",
		gm.ui.DoDefault(moreOptionsButton),
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

	return uiauto.Combine(actionDesc,
		gm.ui.DoDefault(generalSettingsTabButton),
		gm.setToggleValue(finder, value),
	)
}

// SetAdjustVideoLighting sets the option of "Adjust video lighting" in "Video" Tab.
func (gm *GoogleMeet) SetAdjustVideoLighting(value bool) action.Action {
	finder := nodewith.Name("Adjust video lighting").Role(role.Switch).Ancestor(settingsDialog)
	actionDesc := `set "Adjust video lighting" to false`
	if value {
		actionDesc = `set "Adjust video lighting" to true`
	}

	return uiauto.Combine(actionDesc,
		gm.ui.DoDefault(videoSettingsTabButton),
		gm.setToggleValue(finder, value),
	)
}

// SetSendResolution sets the option of "Send Resolution" in "Video" Tab.
func (gm *GoogleMeet) SetSendResolution(value string) action.Action {
	sendResolutionButton := nodewith.NameRegex(regexp.MustCompile("Auto|High definition|Standard definition")).Role(role.ComboBoxMenuButton).Ancestor(settingsDialog).First()
	sendResolutionOption := nodewith.Name(value).Role(role.ListBoxOption).Ancestor(settingsDialog)
	actionDesc := fmt.Sprintf(`set "Send Resolution" to %q`, value)

	return uiauto.Combine(actionDesc,
		gm.ui.DoDefault(videoSettingsTabButton),
		gm.setDropdownValue(sendResolutionButton, sendResolutionOption, value),
	)
}

func (gm *GoogleMeet) setToggleValue(finder *nodewith.Finder, value bool) action.Action {
	return func(ctx context.Context) error {
		shouldBeChecked := checked.False
		resultFinder := finder.Attribute("checked", "false")
		if value {
			shouldBeChecked = checked.True
			resultFinder = finder.Attribute("checked", "true")
		}

		info, err := gm.ui.Info(ctx, finder)
		if err != nil {
			return errors.Wrapf(err, "failed to get node info of %v", finder)
		}
		// Do nothing if the toggle value is already as expected.
		if info.Checked != shouldBeChecked {
			return gm.ui.DoDefaultUntil(
				finder,
				gm.ui.WithTimeout(shortUITimeout).WaitUntilExists(resultFinder),
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

var videoEffectsPageHeading = nodewith.Name("Effects").Role(role.Heading).Ancestor(meetingWebview)

// OpenVideoEffects opens video effects page in GoogleMeet.
func (gm *GoogleMeet) OpenVideoEffects(ctx context.Context) error {
	applyVisualEffectsButton := nodewith.Name("Apply visual effects").Role(role.MenuItem).Ancestor(meetingWebview)

	return uiauto.Combine("open video effects setting dialog",
		gm.ui.DoDefault(moreOptionsButton),
		gm.ui.DoDefaultUntil(applyVisualEffectsButton, gm.ui.WithTimeout(shortUITimeout).WaitUntilExists(videoEffectsPageHeading)),
	)(ctx)
}

// CloseVideoEffects closes the video effects page.
func (gm *GoogleMeet) CloseVideoEffects(ctx context.Context) error {
	closeApplyVisualEffectsButton := nodewith.Name("Close").Role(role.Button).Ancestor(meetingWebview).First()

	return uiauto.Combine("close video effects setting dialog",
		gm.ui.DoDefaultUntil(
			closeApplyVisualEffectsButton,
			gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(videoEffectsPageHeading),
		),
	)(ctx)
}

// SetEffectBlur sets the option of video effects & blur.
func (gm *GoogleMeet) SetEffectBlur(value bool) action.Action {
	blurButtonName := "Turn off visual effects"
	if value {
		blurButtonName = "Blur your background"
	}
	blurButton := nodewith.Name(blurButtonName).Role(role.ToggleButton).Ancestor(meetingWebview)
	return gm.setToggleValue(blurButton, value)
}
