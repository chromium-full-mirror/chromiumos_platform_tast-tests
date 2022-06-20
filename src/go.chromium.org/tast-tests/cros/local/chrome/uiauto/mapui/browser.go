// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mapui

import "go.chromium.org/tast/core/testing"

var browserProfileToolbarButton = testing.RegisterVarString(
	"mapui.browser.avatar_toolbar_button",
	`{"className": "AvatarToolbarButton", "role": "button"}`,
	"UI Node for the Profile Toolbar button in the browser",
)

// BrowserProfileToolbarButton is a node finder for the Avatar Toolbar button in the browser.
var BrowserProfileToolbarButton = nodeFromStringVar(browserProfileToolbarButton)

var browserProfileMenu = testing.RegisterVarString(
	"mapui.browser.profile_menu",
	`{"nameStartingWith": "Accounts and sync", "role": "menu"}`,
	"UI Node for the profile menu of the browser",
)

// BrowserProfileMenu is a node finder for the profile menu of the browser.
var BrowserProfileMenu = nodeFromStringVar(browserProfileMenu)

var browserProfileAddButton = testing.RegisterVarString(
	"mapui.browser.profile_add_button",
	`{"name": "Add", "role": "button"}`,
	"UI Node for the Add button in the profile menu of the browser",
)

// BrowserProfileAddButton is a node finder for the Add button in the profile menu of the browser.
var BrowserProfileAddButton = nodeFromStringVar(browserProfileAddButton)

var browserChooseProfileRoot = testing.RegisterVarString(
	"mapui.browser.choose_profile_root",
	`{"name": "Choose a profile", "role": "rootWebArea"}`,
	"UI Node for the profile chooser dialog",
)

// BrowserChooseProfileRoot is a node finder for the profile chooser dialog.
var BrowserChooseProfileRoot = nodeFromStringVar(browserChooseProfileRoot)

var browserProfileChooserAddButton = testing.RegisterVarString(
	"mapui.browser.profile_chooser_add_button",
	`{"name": "Add", "role": "button"}`,
	"UI Node for the Add button in the profile chooser menu of the browser",
)

// BrowserProfileChooserAddButton is a node finder for the Add button in the profile chooser menu of the browser.
var BrowserProfileChooserAddButton = nodeFromStringVar(browserProfileChooserAddButton)

var browserAddProfileRoot = testing.RegisterVarString(
	"mapui.browser.add_profile_root",
	`{"name": "Set up your new Chrome profile", "role": "rootWebArea"}`,
	"UI Node for the add profile dialog",
)

// BrowserAddProfileRoot is a node finder for the add profile dialog.
var BrowserAddProfileRoot = nodeFromStringVar(browserAddProfileRoot)

var browserAddProfileSigninButton = testing.RegisterVarString(
	"mapui.browser.add_profile_signin_button",
	`{"name": "Sign in", "role": "button", "className": "action-button"}`,
	"UI Node for the Sign in button when adding a new profile in the browser",
)

// BrowserAddProfileSigninButton is a node finder for the Sign in button when adding a new profile in the browser.
var BrowserAddProfileSigninButton = nodeFromStringVar(browserAddProfileSigninButton)

var browserChooseAccountRoot = testing.RegisterVarString(
	"mapui.browser.choose_account_root",
	`{"name": "Choose an account", "role": "rootWebArea"}`,
	"UI Node for the account choosing dialog",
)

// BrowserChooseAccountRoot is a node finder for the account choosing dialog.
var BrowserChooseAccountRoot = nodeFromStringVar(browserChooseAccountRoot)

var browserChooseAccountAddAnotherButton = testing.RegisterVarString(
	"mapui.browser.choose_account_add_another_button",
	`{"name": "Use another account", "role": "button"}`,
	"UI Node for the add another button in the account choosing dialog",
)

// BrowserChooseAccountAddAnotherButton is a node finder for the add another button in the account choosing dialog.
var BrowserChooseAccountAddAnotherButton = nodeFromStringVar(browserChooseAccountAddAnotherButton)

var browserSyncProfileRoot = testing.RegisterVarString(
	"mapui.browser.sync_profile_root",
	`{"name": "Turn on sync", "role": "rootWebArea"}`,
	"UI Node for the profile sync dialog",
)

// BrowserSyncProfileRoot is a node finder for the profile sync dialog.
var BrowserSyncProfileRoot = nodeFromStringVar(browserSyncProfileRoot)

var browserSyncProfileYesButton = testing.RegisterVarString(
	"mapui.browser.sync_profile_yes_button",
	`{"name": "Yes, I'm in", "role": "button"}`,
	"UI Node for the yes button in the profile sync dialog",
)

// BrowserSyncProfileYesButton is a node finder for the yes button in the profile sync dialog.
var BrowserSyncProfileYesButton = nodeFromStringVar(browserSyncProfileYesButton)
