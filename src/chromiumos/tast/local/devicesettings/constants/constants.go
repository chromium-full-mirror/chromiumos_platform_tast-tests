// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package constants contains values used across settings tests.
package constants

import (
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
)

var (
	// KeyboardRow is a finder for the Keyboard subpage in device settings
	KeyboardRow = nodewith.Name("Keyboard").Role(role.GenericContainer)
	// MouseRow is a finder for the Mouse subpage in device settings
	MouseRow = nodewith.Name("Mouse").Role(role.GenericContainer)
	// RemapKeyboardKeys is a finder for the Remap keys  subpage in
	// the keyboard settings page.
	RemapKeyboardKeys = nodewith.Name(
		"Built-in Keyboard Remap keyboard keys").Role(role.GenericContainer)
)

// List of modifier keys.
const (
	Launcher  = "Launcher"
	Search    = "Search"
	Control   = "Ctrl"
	Alt       = "Alt"
	CapsLock  = "Caps Lock"
	Escape    = "Esc"
	Backspace = "Backspace"
	Assistant = "Assistant"
)
