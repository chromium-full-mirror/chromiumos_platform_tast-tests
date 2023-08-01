// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// makeVisibleThenClick makes the node visible then clicks it.
func makeVisibleThenClick(ui *uiauto.Context, node *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("make the node visible then click",
		ui.WaitUntilExists(node),
		ui.MakeVisible(node),
		ui.LeftClick(node),
	)
}

// clickButtonsOnWikipedia returns click actions that can be applied on the Wikipedia.
func clickButtonsOnWikipedia(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	toggleSidebarButton := nodewith.Name("Toggle sidebar").Role(role.Button).Ancestor(activeWindow)
	siteNavigation := nodewith.Name("Site").Role(role.Navigation).Ancestor(activeWindow)
	contributeText := nodewith.Name("Contribute").Role(role.StaticText).Ancestor(siteNavigation)
	searchBox := nodewith.Name("Search Wikipedia").Role(role.SearchBox).Ancestor(activeWindow)
	searchButton := nodewith.Name("Search").Role(role.Button).Ancestor(activeWindow)
	searchButtonTextOffscreen := nodewith.Name("Search").Role(role.StaticText).Offscreen().Ancestor(searchButton)
	return uiauto.NamedCombine("click toggle sidebar button and the search box",
		makeVisibleThenClick(ui, toggleSidebarButton),
		ui.WaitUntilExists(contributeText),
		ui.LeftClick(toggleSidebarButton),
		ui.WaitUntilGone(contributeText),
		makeVisibleThenClick(ui, searchBox),
		// After clicking the search box, the search button will be displayed on the screen.
		ui.WaitUntilGone(searchButtonTextOffscreen),
	)
}

// clickButtonsOnGoogleHelp returns click actions that can be applied on the Google Help.
func clickButtonsOnGoogleHelp(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
	mainMenuButton := nodewith.Name("Main menu").Role(role.Button).Ancestor(activeWindow)
	mainMenuButtonCollapsed := mainMenuButton.Collapsed()
	closeMenuButton := nodewith.Name("Close menu").Role(role.Button).Ancestor(activeWindow)
	textField := nodewith.NameStartingWith("Describe your issue").Role(role.TextFieldWithComboBox).Ancestor(activeWindow)
	return uiauto.NamedCombine("click menu buttons and the search field",
		ui.LeftClick(mainMenuButton),
		ui.LeftClickUntil(closeMenuButton, ui.WithTimeout(5*time.Second).WaitUntilExists(mainMenuButtonCollapsed)),
		ui.LeftClick(textField),
		ui.WaitUntilExists(textField.Focused()),
	)
}

// clickButtonsOnLocalWebsite returns click actions that can be applied on the local website.
func clickButtonsOnLocalWebsite(ui *uiauto.Context, activeWindow *nodewith.Finder) uiauto.Action {
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

// clickButtonsOnYTMusic returns click actions that can be applied on the YouTube Music.
func clickButtonsOnYTMusic(ui *uiauto.Context, kb *input.KeyboardEventWriter) uiauto.Action {
	// The YouTube Music window changes the name after the ad finishes, specifying the
	// nodes with the root web area to avoid errors caused by the window name change.
	youtubeMusicRootWebArea := nodewith.NameContaining("YouTube Music").Role(role.RootWebArea)
	searchField := nodewith.NameStartingWith("Search").Role(role.TextFieldWithComboBox).Ancestor(youtubeMusicRootWebArea)
	searchButton := nodewith.Name("Initiate search").Role(role.Button).Ancestor(youtubeMusicRootWebArea)
	backButton := nodewith.Name("Back").Role(role.Button).Ancestor(youtubeMusicRootWebArea)
	return uiauto.NamedCombine("click search, back buttons and the search field",
		ui.LeftClick(searchButton),
		// Sometimes it fails to click the backButton in lacros-chrome, instead pressing "Esc" exits the search field.
		uiauto.IfFailThen(
			ui.LeftClickUntil(backButton, ui.WithTimeout(5*time.Second).WaitUntilExists(searchButton)),
			uiauto.NamedAction(`press "Esc" key`, kb.AccelAction("Esc")),
		),
		ui.LeftClick(searchButton),
		ui.LeftClick(searchField),
		ui.WaitUntilExists(searchField.Focused()),
	)
}

// GenerateEventLatency clicks buttons, types and deletes the text on the given website to generate corresponding EventLatency metrics.
// Only Wikipedia, Google Help, YouTube Music, and the local website from spera.TabSwitch tests are supported by this function.
func GenerateEventLatency(ctx context.Context, tconn *chrome.TestConn, webName string) error {
	window, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get active window")
	}
	activeWindow := nodewith.Name(window.Title).Role(role.Window).HasClass(window.Name)
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard")
	}
	defer kb.Close(ctx)

	var clickButtonsOnWebsite uiauto.Action
	switch webName {
	case "Wikipedia":
		clickButtonsOnWebsite = clickButtonsOnWikipedia(ui, activeWindow)
	case "Google Help":
		clickButtonsOnWebsite = clickButtonsOnGoogleHelp(ui, activeWindow)
	case "localWebsite":
		clickButtonsOnWebsite = clickButtonsOnLocalWebsite(ui, activeWindow)
	case "YouTube Music":
		clickButtonsOnWebsite = clickButtonsOnYTMusic(ui, kb)
	default:
		return errors.Errorf("unsupported website: %s", webName)
	}

	typeAndDeleteActions := uiauto.NamedCombine("type and delete the text",
		kb.TypeAction("Chromebook"),
		kb.AccelAction("Ctrl+A"),
		kb.AccelAction("Backspace"),
	)

	return uiauto.Combine("click buttons then type and delete the text",
		clickButtonsOnWebsite,
		typeAndDeleteActions,
	)(ctx)
}
