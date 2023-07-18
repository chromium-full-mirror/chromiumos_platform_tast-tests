// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package imesettings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// EmojiSuggestionsOption represents the option name of Emoji suggestions toggle option.
const EmojiSuggestionsOption = "Emoji suggestions"

// AddInputMethodInOSSettings returns a user action adding certain input method in OS settings.
func AddInputMethodInOSSettings(uc *useractions.UserContext, kb *input.KeyboardEventWriter, im ime.InputMethod) uiauto.Action {
	action := func(ctx context.Context) error {
		// Use the first 5 letters to search input method.
		// This will handle Unicode characters correctly.
		runes := []rune(im.Name)
		searchKeyword := string(runes[0:5])

		settings, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch OS settings and land at inputs setting page")
		}
		return uiauto.Combine("add input method",
			settings.ClickAddInputMethodButton(),
			settings.SearchInputMethod(kb, searchKeyword, im.Name),
			settings.SelectInputMethod(im.Name),
			settings.ClickAddButtonToConfirm(),
			im.WaitUntilInstalled(uc.TestAPIConn()),
			settings.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		"Add input method in OS Settings",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMEManagement,
				useractions.AttributeTestScenario: fmt.Sprintf("Add input method %q", im.Name),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// RemoveInputMethodInOSSettings returns a user action removing certain input method in OS settings.
func RemoveInputMethodInOSSettings(uc *useractions.UserContext, im ime.InputMethod) uiauto.Action {
	action := func(ctx context.Context) error {
		settings, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch OS settings and land at inputs setting page")
		}
		return uiauto.Combine("remove input method",
			settings.RemoveInputMethod(im.Name),
			im.WaitUntilRemoved(uc.TestAPIConn()),
			func(ctx context.Context) error {
				activeInputMethod, err := ime.ActiveInputMethod(ctx, uc.TestAPIConn())
				if err != nil {
					return errors.Wrap(err, "failed to get active input method")
				}
				uc.SetAttribute(useractions.AttributeInputMethod, activeInputMethod.Name)
				return nil
			},
			settings.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		"Remove input method in OS Settings",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMEManagement,
				useractions.AttributeTestScenario: fmt.Sprintf("Remove input method %q", im.Name),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// SetEmojiSuggestions returns a user action to change 'Emoji suggestions' setting.
func SetEmojiSuggestions(uc *useractions.UserContext, isEnabled bool) uiauto.Action {
	actionName := "Enable emoji suggestions in OS settings"
	if !isEnabled {
		actionName = "Disable emoji suggestions in OS settings"
	}

	action := func(ctx context.Context) error {
		settings, err := LaunchAtSuggestionSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch OS settings and land at inputs setting page")
		}
		return uiauto.Combine("toggle setting and close page",
			settings.SetToggleOption(uc.Chrome(), EmojiSuggestionsOption, isEnabled),
			settings.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		actionName,
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureEmojiSuggestion,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// SetGlideTyping returns a user action to change 'Glide suggestions' setting.
func SetGlideTyping(uc *useractions.UserContext, im ime.InputMethod, isEnabled bool) uiauto.Action {
	actionName := "Enable glide typing in IME setting"
	if !isEnabled {
		actionName = "Disable glide typing in IME setting"
	}

	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}
		return uiauto.Combine("change glide typing setting",
			setting.OpenInputMethodSetting(uc.TestAPIConn(), im),
			setting.ToggleGlideTyping(uc.Chrome(), isEnabled),
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		actionName,
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureGlideTyping,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// AutoCorrectionLevel describes the auto correction level of an input method.
type AutoCorrectionLevel bool

// Available auto correction levels.
const (
	AutoCorrectionOff AutoCorrectionLevel = false
	AutoCorrectionOn                      = true
)

// SetPKAutoCorrection sets the PK auto-correction toggle to On or Off as required.
func SetPKAutoCorrection(uc *useractions.UserContext, im ime.InputMethod, acLevel AutoCorrectionLevel) uiauto.Action {
	actionName := fmt.Sprintf("Set PK auto-correction level to %t", acLevel)

	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}
		return uiauto.Combine(actionName,
			setting.OpenInputMethodSetting(uc.TestAPIConn(), im),
			setting.setPKAutoCorrection(uc.Chrome(), bool(acLevel)),
			// TODO(b/157686038) A better solution to identify decoder status.
			// Decoder works async in returning status to frontend IME and self loading.
			uiauto.Sleep(5*time.Second),
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		actionName,
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureAutoCorrection,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// SetVKAutoCapitalization returns a user action to change 'On-screen keyboard Auto-capitalization' setting.
func SetVKAutoCapitalization(uc *useractions.UserContext, im ime.InputMethod, isEnabled bool) uiauto.Action {
	actionName := "Enable VK auto capitalization in IME setting"
	if !isEnabled {
		actionName = "Disable VK auto capitalization in IME setting"
	}

	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}
		return uiauto.Combine(actionName,
			setting.OpenInputMethodSetting(uc.TestAPIConn(), im),
			setting.ToggleAutoCap(uc.Chrome(), isEnabled),
			// TODO(b/157686038) A better solution to identify decoder status.
			// Decoder works async in returning status to frontend IME and self loading.
			uiauto.Sleep(5*time.Second),
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		actionName,
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureAutoCapitalization,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// EnableInputOptionsInShelf returns a user action to change IME setting
// to show / hide IME options in shelf.
func EnableInputOptionsInShelf(uc *useractions.UserContext, shown bool) uiauto.Action {
	ui := uiauto.New(uc.TestAPIConn())
	imeMenuTrayButtonFinder := nodewith.Name("IME menu button").Role(role.Button)

	userActionName := "Show input options in the shelf"
	if !shown {
		userActionName = "Hide input options in the shelf"
	}

	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}

		return uiauto.Combine(strings.ToLower(userActionName),
			setting.ToggleShowInputOptionsInShelf(uc.Chrome(), shown),
			func(ctx context.Context) error {
				if shown {
					return ui.WaitUntilExists(imeMenuTrayButtonFinder)(ctx)
				}
				return ui.WaitUntilGone(imeMenuTrayButtonFinder)(ctx)
			},
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		userActionName,
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeatureIMEManagement,
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// SetKoreanKeyboardLayout returns a user action to change 'Korean keyboard layout' setting.
func SetKoreanKeyboardLayout(uc *useractions.UserContext, keyboardLayout string) uiauto.Action {
	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}

		return uiauto.Combine("test input method settings change",
			setting.OpenInputMethodSetting(uc.TestAPIConn(), ime.Korean),
			setting.ChangeKoreanKeyboardLayout(uc.Chrome(), keyboardLayout),
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		"Change Korean keyboard layout setting",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMESpecific,
				useractions.AttributeTestScenario: fmt.Sprintf("Change Korean keyboard layout to %q", keyboardLayout),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

// SetKoreanInputASyllableAtATime returns a user action to change 'Input a syllable at a time' setting.
func SetKoreanInputASyllableAtATime(uc *useractions.UserContext, value bool) uiauto.Action {
	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}

		return uiauto.Combine("test input method settings change",
			setting.OpenInputMethodSetting(uc.TestAPIConn(), ime.Korean),
			setting.ChangeKoreanInputASyllableAtATime(uc.Chrome(), value),
			setting.Close,
		)(ctx)
	}

	return uiauto.UserAction(
		"Change Korean input a syllable at a time",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMESpecific,
				useractions.AttributeTestScenario: fmt.Sprintf("Change input a syllable at a time to %t", value),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}

func closeJapaneseSettings(uc *useractions.UserContext) uiauto.Action {
	const url = "chrome-extension://jkghodnilhceideoidjikpgommlajknk/mozc_option.html"
	return func(ctx context.Context) error {
		if err := uc.TestAPIConn().Call(ctx, nil, `async (url) => {
                  const query = tast.promisify(chrome.tabs.query);
                  const remove = tast.promisify(chrome.tabs.remove);
                  const tabs = await query({ url });
                  console.error(tabs)
                  // Works for any number of tabs, even if it will usually be 1.
                  await Promise.all(tabs.map(t => remove(t.id)));
          }`, url); err != nil {
			return errors.Wrapf(err, "failed to close tab %q", url)
		}
		return nil
	}
}

// SetJapaneseKeyboardSettings returns a user action to open the Japanese input settings and run the specified action to change the settings.
// The input method should either be Japanese or Japanese with US keyboard
func SetJapaneseKeyboardSettings(uc *useractions.UserContext, ui *uiauto.Context, im ime.InputMethod, settingAction uiauto.Action) uiauto.Action {
	action := func(ctx context.Context) error {
		setting, err := LaunchAtInputsSettingsPage(ctx, uc.TestAPIConn(), uc.Chrome())
		if err != nil {
			return errors.Wrap(err, "failed to launch input settings")
		}

		return uiauto.Combine("test input method settings change",
			setting.OpenInputMethodSetting(uc.TestAPIConn(), im),
			settingAction,
			setting.Close,
			closeJapaneseSettings(uc),
		)(ctx)
	}

	return uiauto.UserAction(
		"Change Japanese keyboard setting",
		action,
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature:      useractions.FeatureIMESpecific,
				useractions.AttributeTestScenario: fmt.Sprintf("Change Japanese keyboard settings"),
			},
			Tags: []useractions.ActionTag{useractions.ActionTagEssentialInputs},
		},
	)
}
