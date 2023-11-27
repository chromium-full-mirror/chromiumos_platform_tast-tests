// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testserver

import (
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome/ime/emojipicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
)

var emojiMenuFinder = nodewith.NameStartingWith("Emoji")

// TriggerEmojiPickerFromContextMenu launches emoji picker from context menu.
func (its *InputsTestServer) TriggerEmojiPickerFromContextMenu(inputField InputField) uiauto.Action {
	return uiauto.Combine("launches emoji picker from context menu",
		its.RightClickFieldAndWaitForActive(inputField),
		its.ui.LeftClick(emojiMenuFinder),
		emojipicker.WaitUntilExists(its.tconn),
	)
}

// DismissGifNudgeOverlay returns a user action to dismiss gif nudge overlay in PK emoji picker.
func (its *InputsTestServer) DismissGifNudgeOverlay() uiauto.Action {
	return uiauto.Combine("Dismiss gif nudge overlay",
		its.ui.LeftClickUntil(emojipicker.SearchFieldFinder, its.ui.WithTimeout(5*time.Second).WaitUntilGone(emojipicker.NudgeOverlay)),
	)
}

// InputEmojiWithEmojiPicker returns a user action to input Emoji with PK emoji picker on E14s test server.
func (its *InputsTestServer) InputEmojiWithEmojiPicker(uc *useractions.UserContext, inputField InputField, emojiChar string) uiauto.Action {
	emojiCharFinder := emojipicker.NodeFinder.Name(emojiChar).First()
	ui := emojipicker.NewUICtx(its.tconn)

	action := uiauto.Combine(fmt.Sprintf("input emoji with emoji picker on field %v", inputField),
		its.Clear(inputField),
		its.TriggerEmojiPickerFromContextMenu(inputField),
		its.DismissGifNudgeOverlay(),
		// Select item from emoji picker.
		ui.LeftClick(emojiCharFinder),
		// Wait for input value to test emoji.
		util.WaitForFieldTextToBe(uc.TestAPIConn(), inputField.Finder(), emojiChar),
	)

	return uiauto.UserAction(
		"Input Emoji with Emoji Picker",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeTestScenario: fmt.Sprintf("Input %q into %q field via emoji picker", emojiChar, string(inputField)),
			},
			Tags: []useractions.ActionTag{
				useractions.ActionTagEssentialInputs,
			},
		})
}

// InputEmojiWithEmojiPickerSearch returns a user action to input Emoji with PK emoji picker on E14s test server using search.
func (its *InputsTestServer) InputEmojiWithEmojiPickerSearch(uc *useractions.UserContext, inputField InputField, keyboard *input.KeyboardEventWriter, searchString, emojiChar string) uiauto.Action {
	emojiResultFinder := nodewith.NameStartingWith(emojiChar).First()
	ui := emojipicker.NewUICtx(its.tconn)

	action := uiauto.Combine(fmt.Sprintf("input emoji with emoji picker on field %v", inputField),
		its.Clear(inputField),
		its.TriggerEmojiPickerFromContextMenu(inputField),
		ui.LeftClick(emojipicker.SearchFieldFinder),
		keyboard.TypeAction(searchString),
		util.WaitForFieldEmpty(its.tconn, emojipicker.NodeFinder.Name("😄").First()),
		ui.LeftClickUntil(emojiResultFinder, ui.WithTimeout(time.Second).WaitUntilGone(emojipicker.RootFinder)),
		// Wait for input value to be test Emoji.
		util.WaitForFieldTextToBe(uc.TestAPIConn(), inputField.Finder(), emojiChar),
	)

	return uiauto.UserAction(
		"Input Emoji by search in Emoji Picker",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeTestScenario: fmt.Sprintf("Search %q for %q then submit", searchString, emojiChar),
			},
			Tags: []useractions.ActionTag{
				useractions.ActionTagEssentialInputs,
			},
		})
}

// InputGifWithEmojiPickerSearch returns a user action to input GIF with PK emoji picker on E14S test server using search.
func (its *InputsTestServer) InputGifWithEmojiPickerSearch(uc *useractions.UserContext, inputField InputField, keyboard *input.KeyboardEventWriter, searchString string) uiauto.Action {

	emojiResultFinder := emojipicker.NodeFinder.Role(role.Button).NameContaining("GIF").First()
	ui := emojipicker.NewUICtx(its.tconn)

	action := uiauto.Combine(fmt.Sprintf("input emoji with emoji picker on field %v", inputField),
		its.Clear(inputField),
		its.TriggerEmojiPickerFromContextMenu(inputField),
		its.DismissGifNudgeOverlay(),
		ui.LeftClickUntil(emojipicker.SearchFieldFinder, ui.WithTimeout(time.Second).WaitUntilExists(emojipicker.SearchFieldFinder)),
		keyboard.TypeAction(searchString),
		ui.WithTimeout(time.Second).WaitUntilExists(emojiResultFinder),
		ui.LeftClickUntil(emojiResultFinder, ui.WithTimeout(time.Second).WaitUntilGone(emojipicker.RootFinder)),
		its.WaitUntilFieldContainsImages(inputField),
	)

	return uiauto.UserAction(
		"Input GIF by search in Emoji Picker",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureEmojiPicker,
				useractions.AttributeInputField:   string(inputField),
				useractions.AttributeTestScenario: fmt.Sprintf("Search %q for GIF then submit", searchString),
			},
			Tags: []useractions.ActionTag{
				useractions.ActionTagEssentialInputs,
			},
		})
}
