// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diagnosticsapp contains drivers for controlling the ui of diagnostics SWA.
package diagnosticsapp

import (
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
)

// regionalKeys defines keys which specified by region.
var regionalKeys = map[string][]string{
	"us": {"esc", "backspace", "shift", "alt", "ctrl"},
	"jp": {"あ", "ほ", "ゆ", "英数", "かな"},
	"fr": {"échap", "é", "ù", "◌̂", "alt gr"},
}

// DxInternalKeyboardTestButtons defines test button for internal keyboard which specified by region.
var DxInternalKeyboardTestButtons = map[string]*nodewith.Finder{
	"us": DxInternalKeyboardTestButton,
	"jp": nodewith.NameStartingWith("テスト").Role(role.Button).Nth(1),
	"fr": nodewith.NameContaining("Tester").Role(role.Button).First(),
}

// DxKeyboardTabButtons defines keyboard tab button which specified by region.
var DxKeyboardTabButtons = map[string]*nodewith.Finder{
	"us": DxKeyboardTab,
	"jp": nodewith.NameContaining("キーボード").Role(role.GenericContainer),
	"fr": nodewith.NameContaining("Clavier").Role(role.GenericContainer),
}
