// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
)

var clickActions = map[string]func(*uiauto.Context, *nodewith.Finder) uiauto.Action{
	"Wikipedia":     wikiClickActions,
	"Google Help":   googleHelpClickActions,
	"localWebsite":  localWebsiteClickActions,
	"YouTube Music": ytMusicClickActions,
}

// makeVisibleThenClick makes the node visible then clicks it.
func makeVisibleThenClick(ui *uiauto.Context, node *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("make the node visible then click",
		ui.WaitUntilExists(node),
		ui.MakeVisible(node),
		ui.LeftClick(node),
	)
}

// wikiClickActions returns click actions that can be applied on the Wikipedia.
func wikiClickActions(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	languageSettingsButton := nodewith.Name("Language settings").Role(role.Button).Ancestor(activeWindow)
	languageSettingsHeading := nodewith.Name("Language settings").Role(role.Heading).Ancestor(activeWindow)
	searchBox := nodewith.Name("Search Wikipedia").Role(role.SearchBox).Ancestor(activeWindow)
	return uiauto.NamedCombine("click language settings button and the search box",
		makeVisibleThenClick(ui, languageSettingsButton),
		ui.WaitUntilExists(languageSettingsHeading),
		ui.LeftClick(languageSettingsButton),
		ui.WaitUntilGone(languageSettingsHeading),
		makeVisibleThenClick(ui, searchBox),
		ui.WaitUntilExists(searchBox.Focused()),
	)
}

// googleHelpClickActions returns click actions that can be applied on the Google Help.
func googleHelpClickActions(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	mainMenuButton := nodewith.Name("Main menu").Role(role.Button).Ancestor(activeWindow)
	closeMenuButton := nodewith.Name("Close menu").Role(role.Button).Ancestor(activeWindow)
	textField := nodewith.NameStartingWith("Describe your issue").Role(role.TextFieldWithComboBox).Ancestor(activeWindow)
	return uiauto.NamedCombine("click menu buttons and the search field",
		ui.LeftClick(mainMenuButton),
		ui.LeftClick(closeMenuButton),
		ui.WaitUntilExists(mainMenuButton.Collapsed()),
		ui.LeftClick(textField),
		ui.WaitUntilExists(textField.Focused()),
	)
}

// localWebsiteClickActions returns click actions that can be applied on the local website.
func localWebsiteClickActions(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	hideTextButton := nodewith.Name("Hide text").Role(role.Button).Ancestor(activeWindow)
	showTextButton := nodewith.Name("Show text").Role(role.Button).Ancestor(activeWindow)
	textField := nodewith.Name("Input your text:").Role(role.TextField).Ancestor(activeWindow)
	return uiauto.NamedCombine("click text buttons and the text field",
		ui.LeftClick(hideTextButton),
		ui.LeftClick(showTextButton),
		ui.WaitUntilExists(hideTextButton),
		ui.LeftClick(textField),
		ui.WaitUntilExists(textField.Focused()),
	)
}

// ytMusicClickActions returns click actions that can be applied on the YouTube Music.
func ytMusicClickActions(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	// The YouTube Music window changes the name after the ad finishes, specifying the
	// nodes with the root web area to avoid errors caused by the window name change.
	youtubeMusicRootWebArea := nodewith.NameContaining("YouTube Music").Role(role.RootWebArea)
	searchField := nodewith.Name("Search").Role(role.TextFieldWithComboBox).Ancestor(youtubeMusicRootWebArea)
	searchButton := nodewith.Name("Initiate search").Role(role.Button).Ancestor(youtubeMusicRootWebArea)
	backButton := nodewith.Name("Back").Role(role.Button).Ancestor(youtubeMusicRootWebArea)
	return uiauto.NamedCombine("click search, back buttons and the search field",
		ui.LeftClick(searchButton),
		ui.LeftClick(backButton),
		ui.WaitUntilExists(searchButton),
		ui.LeftClick(searchButton),
		ui.LeftClick(searchField),
		ui.WaitUntilExists(searchField.Focused()),
	)
}

// GenerateEventLatency clicks buttons, types and deletes the text on the given website to generate corresponding EventLatency metrics.
// Only Wikipedia, Google Help, YouTube Music, and the local website from spera.TabSwitch tests are supported by this function.
func GenerateEventLatency(ctx context.Context, tconn *chrome.TestConn, webName string) error {
	clickActions, ok := clickActions[webName]
	if !ok {
		return errors.Errorf("unsupported website: %s", webName)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard")
	}
	defer kb.Close()

	typeAndDeleteActions := uiauto.NamedCombine("type and delete the text",
		kb.TypeAction("Chromebook"),
		kb.AccelAction("Ctrl+A"),
		kb.AccelAction("Backspace"),
	)

	window, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get active window")
	}
	activeWindow := nodewith.Name(window.Title).Role(role.Window).HasClass(window.Name)

	return uiauto.Combine("click buttons then type and delete the text",
		clickActions(uiauto.New(tconn), activeWindow),
		typeAndDeleteActions,
	)(ctx)
}
