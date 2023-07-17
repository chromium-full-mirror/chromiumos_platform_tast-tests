// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package imesettings supports managing input methods in OS settings.
package imesettings

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const inputsSubPageURL = "osLanguages/input"

var defaultPollOpts = testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}

var addInputMethodButton = nodewith.Name("Add input methods").Role(role.Button)
var searchInputMethodField = nodewith.Name("Search by language or input name").Role(role.SearchBox)
var deleteWordButton = nodewith.Name("Delete word").HasClass("icon-clear").Role(role.Button)

// settingOption represents an IME setting item shown in OS Settings.
type settingOption string

// Available IME setting items.
const (
	GlideTyping                            settingOption = "Enable glide typing"
	AutoCapitalization                     settingOption = "Auto-capitalization"
	ShowInputOptionsInShelf                settingOption = "Show input options in the shelf"
	KoreanKeyboardLayout                   settingOption = "Korean keyboard layout"
	AutoCorrection                         settingOption = "Auto-correction"
	SpellingGrammarCheck                   settingOption = "Spelling and grammar check"
	SpellCheck                             settingOption = "Spell check"
	JapaneseInputMode                      settingOption = "Input mode"
	JapanesePunctuationStyle               settingOption = "Punctuation style"
	JapaneseSymbolStyle                    settingOption = "Symbol style"
	JapaneseSpaceInputStyle                settingOption = "Space input style"
	JapaneseSelectionShortcut              settingOption = "Selection shortcut"
	JapaneseKeymapStyle                    settingOption = "Keymap style"
	JapaneseAutomaticallySwitchToHalfwidth settingOption = "Automatically switch to halfwidth"
	JapaneseShiftKeyModeSwitch             settingOption = "Shift key mode switch"
	JapaneseUseInputHistory                settingOption = "Use input history"
	JapaneseUseSystemDictionary            settingOption = "Use system dictionary"
	JapaneseNumberOfSuggestions            settingOption = "Number of suggestions"
)

// IMESettings is a wrapper around the settings app used to control the inputs settings page.
type IMESettings struct {
	*ossettings.OSSettings
}

// New returns a new IME settings context.
func New(tconn *chrome.TestConn) *IMESettings {
	return &IMESettings{ossettings.New(tconn)}
}

// LaunchAtInputsSettingsPage launches Settings app at inputs setting page.
func LaunchAtInputsSettingsPage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*IMESettings, error) {
	ui := uiauto.New(tconn)
	inputsHeading := nodewith.NameStartingWith("Inputs").Role(role.Heading).Ancestor(ossettings.WindowFinder)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, inputsSubPageURL, ui.Exists(inputsHeading))
	if err != nil {
		return nil, err
	}
	return &IMESettings{settings.WithPollOpts(defaultPollOpts)}, nil
}

// ClickAddInputMethodButton returns a function that clicks AddInputMethod button in inputs setting page.
func (i *IMESettings) ClickAddInputMethodButton() uiauto.Action {
	return i.LeftClick(addInputMethodButton)
}

// SearchInputMethod returns a function that searches input method by typing keyboard into searchbox.
// SearchInputMethod also waits for expected IME displayed on screen.
func (i *IMESettings) SearchInputMethod(kb *input.KeyboardEventWriter, searchKeyword, inputMethodName string) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("SearchInputMethod(%s, %s)", searchKeyword, inputMethodName),
		i.LeftClick(searchInputMethodField),
		kb.TypeAction(searchKeyword),
		i.WaitUntilExists(nodewith.Name(inputMethodName).Role(role.CheckBox).Onscreen()),
	)
}

// SelectInputMethod returns a function that selects an input method by displayed name.
func (i *IMESettings) SelectInputMethod(inputMethodName string) uiauto.Action {
	inputMethodOption := nodewith.Name(inputMethodName).Role(role.CheckBox)
	return uiauto.Combine(fmt.Sprintf("SelectInputMethod(%s)", inputMethodName),
		i.MakeVisible(inputMethodOption),
		i.LeftClick(inputMethodOption),
	)
}

// ClickAddButtonToConfirm returns a function that clicks Add button to confirm adding one or more input methods.
func (i *IMESettings) ClickAddButtonToConfirm() uiauto.Action {
	return i.LeftClick(nodewith.Name("Add").Role(role.Button))
}

// RemoveInputMethod returns a function that removes the input method by clicking cross button next to the input method on UI.
func (i *IMESettings) RemoveInputMethod(inputMethodName string) uiauto.Action {
	return i.LeftClick(nodewith.Name("Remove " + inputMethodName).Role(role.Button))
}

// OpenInputMethodSetting opens the input method setting page in OS settings.
// The setting button is named as "Open settings page for " + im.Name.
// Japanese is the only exemption in "IME settings in the OS setting".
func (i *IMESettings) OpenInputMethodSetting(tconn *chrome.TestConn, im ime.InputMethod) uiauto.Action {
	return func(ctx context.Context) error {
		var imeSettingHeading *nodewith.Finder
		// Japanese input settings page has not been migrated to OS Settings yet.
		// Remove this special case once it is migrated.
		if im.Equal(ime.JapaneseWithUSKeyboard) || im.Equal(ime.Japanese) {
			imeSettingHeading = nodewith.Name("Japanese input settings").Role(role.Heading)
		} else {
			imeSettingHeading = nodewith.Name(im.Name).Role(role.Heading).Ancestor(ossettings.WindowFinder)
		}

		imSettingButton := nodewith.Name("Open settings page for " + im.Name)
		successCondition := uiauto.New(tconn).WithTimeout(5 * time.Second).WaitUntilExists(imeSettingHeading)
		return i.LeftClickUntil(imSettingButton, successCondition)(ctx)
	}
}

// ToggleShowInputOptionsInShelf clicks the 'Show input options in the shelf' toggle button to enable/disable the setting.
func (i *IMESettings) ToggleShowInputOptionsInShelf(cr *chrome.Chrome, expected bool) uiauto.Action {
	return i.SetToggleOption(cr, string(ShowInputOptionsInShelf), expected)
}

// ToggleGlideTyping clicks the 'Enable glide typing' toggle button to enable/disable the setting.
func (i *IMESettings) ToggleGlideTyping(cr *chrome.Chrome, expected bool) uiauto.Action {
	return i.SetToggleOption(cr, string(GlideTyping), expected)
}

// ToggleAutoCap clicks the 'Auto-capitalization' toggle button to enable/disable the setting.
func (i *IMESettings) ToggleAutoCap(cr *chrome.Chrome, expected bool) uiauto.Action {
	return i.SetToggleOption(cr, string(AutoCapitalization), expected)
}

// SetSpellingAndGrammarCheck clicks the spell check toggle button to enable/disable the setting.
func (i *IMESettings) SetSpellingAndGrammarCheck(cr *chrome.Chrome, expected bool) uiauto.Action {
	return func(ctx context.Context) error {
		// The name of this option could be "Spelling and grammar check" or "Spell check", retrieves its name first is essential.
		nameRegex := regexp.MustCompile(fmt.Sprintf("(%s)|(%s)", SpellCheck, SpellingGrammarCheck))
		spellChecToggleButton := nodewith.NameRegex(nameRegex).Role(role.ToggleButton)
		info, err := i.Info(ctx, spellChecToggleButton)
		if err != nil {
			return errors.Wrap(err, "failed to check node info")
		}

		return i.SetToggleOption(cr, info.Name, expected)(ctx)
	}
}

// AddCustomizedSpellCheck adds customized word to spelling check dictionary.
func (i *IMESettings) AddCustomizedSpellCheck(cr *chrome.Chrome, kb *input.KeyboardEventWriter, word string) uiauto.Action {
	addWordsTextField := nodewith.Name("Add words you want spell check to skip").Role(role.TextField)
	return func(ctx context.Context) error {
		if err := i.NavigateToPageURL(ctx, cr, "osLanguages/editDictionary", i.WaitUntilExists(addWordsTextField)); err != nil {
			return errors.Wrap(err, "failed to navigate to customize spell check page")
		}

		return uiauto.Combine("add word into customize spell check",
			i.FocusAndWait(addWordsTextField),
			kb.TypeAction(word),
			i.LeftClick(nodewith.Name("Add word").Role(role.Button)),
			// The added word doesn't have a name related to the word.
			// Verify that the word is added by checking the node named `Delete word` exists.
			i.WaitUntilExists(deleteWordButton),
		)(ctx)
	}
}

// RemoveCustomizedSpellCheck removes customized word from spelling check dictionary by clicking
// the `Delete word` node until the node gone to make sure the word is removed.
func (i *IMESettings) RemoveCustomizedSpellCheck() uiauto.Action {
	return i.LeftClickUntil(deleteWordButton, i.Gone(deleteWordButton))
}

// ChangeKoreanKeyboardLayout sets the Korean keyboard layout to a specific value.
func (i *IMESettings) ChangeKoreanKeyboardLayout(cr *chrome.Chrome, expected string) uiauto.Action {
	return func(ctx context.Context) error {
		currentLayout, err := i.DropdownValue(ctx, cr, string(KoreanKeyboardLayout))
		if err != nil {
			return errors.Wrap(err, "failed to get current layout value")
		}
		if currentLayout == expected {
			testing.ContextLogf(ctx, "Skip to change Korean keyboard layout: the current layout is already %q", expected)
			return nil
		}
		testing.ContextLogf(ctx, "Changing keyboard layout from %q to %q", currentLayout, expected)
		return i.SetDropDownOption(cr, string(KoreanKeyboardLayout), expected)(ctx)
	}
}

// setPKAutoCorrection sets the 'Auto-correction' of PK setting to a specific
// value.
func (i *IMESettings) setPKAutoCorrection(cr *chrome.Chrome, expected bool) uiauto.Action {
	return func(ctx context.Context) error {
		// TODO(b/261653159): This only works for PK at the moment, if this function
		// is used for vk in the future as well, consider updating
		// IsToggleOptionEnabled to not always match first result.
		if isEnabled, err := i.IsToggleOptionEnabled(ctx, cr, string(AutoCorrection)); err != nil {
			return err
		} else if isEnabled == expected {
			return nil
		}
		optionFinder := nodewith.Name(string(AutoCorrection)).First()
		return i.LeftClick(optionFinder)(ctx)
	}
}

// SetJapaneseDropdown sets a dropdown in the Japanese settings page to the specified value.
func SetJapaneseDropdown(ui *uiauto.Context, setting settingOption, value string) uiauto.Action {
	dropdownFinder := nodewith.Name(string(setting)).Role(role.ComboBoxSelect)
	dropdownItemFinder := nodewith.Name(value).Role(role.ListBoxOption)
	return uiauto.Combine("set drop down option",
		ui.LeftClick(dropdownFinder),
		ui.WaitUntilExists(dropdownItemFinder),
		ui.MakeVisible(dropdownItemFinder),
		ui.LeftClick(dropdownItemFinder),
	)
}

// SetJapaneseCheckbox sets a checkbox in the Japanese settings page to the specified value.
func SetJapaneseCheckbox(ui *uiauto.Context, setting settingOption, value checked.Checked) uiauto.Action {
	checkboxFinder := nodewith.Name(string(setting)).Role(role.CheckBox)
	return func(ctx context.Context) error {
		info, err := ui.Info(ctx, checkboxFinder)
		if err != nil {
			return errors.Wrap(err, "failed to get checkbox value")
		}
		if info.Checked == value {
			testing.ContextLogf(ctx, "Skip to change %q: the current value is already %q", setting, value)
			return nil
		}
		return ui.LeftClick(checkboxFinder)(ctx)
	}
}
