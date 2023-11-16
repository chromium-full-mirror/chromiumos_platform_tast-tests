// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package constants contains values used across settings tests.
package constants

import (
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

var (
	// KeyboardRow is a finder for the Keyboard subpage in device settings.
	KeyboardRow = nodewith.Name("Keyboard").Role(role.GenericContainer)
	// MouseRow is a finder for the Mouse subpage in device settings.
	MouseRow = nodewith.Name("Mouse").Role(role.GenericContainer)
	// TouchpadRow is a finder for the Touchpad subpage in device settings.
	TouchpadRow = nodewith.Name("Touchpad").Role(role.GenericContainer)
	// TrackPointRow is a finder for the TrackPoint subpage in device settings.
	TrackPointRow = nodewith.Name("TrackPoint").Role(role.GenericContainer)
	// CustomizeKeyboardKeys is a finder for the built-in keyboard Remap keys subpage in
	// the keyboard settings page.
	CustomizeKeyboardKeys = nodewith.ClassName(
		"hr bottom-divider remap-keyboard-keys-row-internal").Role(role.GenericContainer)
	// OverviewMode is the a finder for to confirm if the over view mode is enabled.
	OverviewMode = nodewith.Name("Desk: Desk 1").Role(role.Button)
	// Dropdown is the finder for the action drop down list in the customize mouse button page.
	Dropdown = nodewith.ClassName("md-select").Role(role.ComboBoxSelect)
)

// List of modifier keys.
const (
	Launcher  = "launcher"
	Search    = "search"
	Control   = "ctrl"
	Alt       = "alt"
	CapsLock  = "caps Lock"
	Escape    = "escape"
	Backspace = "backspace"
	Assistant = "assistant"
)

// Delete six pack key constants.
const (
	DeleteSixPackKey     = "delete"
	DeleteSearchShortcut = "search + backspace"
	DeleteAltShortcut    = "alt + backspace"
	Off                  = "Off"
)

// Fkey constants.
const (
	F11              = "F11"
	F11ShiftShortcut = "shift + back"
	F12              = "F12"
	F12AltShortcut   = "alt + forward"
)

// Button or node names in Mouse device section ui.
const (
	MouseLabel  	= "Tast virtual mouse"
	MiddleButton    = "Middle Button"
	OtherButton     = "Other Button 1"
	EditButton      = "edit-button"
	SaveButton 	    = "Save"
	RenamedButton1  = "RenamedButton 1"
	RenamedButton2  = "RenamedButton 2"
)