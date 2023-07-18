// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diagnosticsapp contains drivers for controlling the ui of diagnostics SWA.
package diagnosticsapp

import (
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// regionalKeys defines keys which specified by region.
var regionalKeys = map[string][]string{
	"us": {"esc", "backspace", "shift", "alt", "ctrl"},
	"jp": {"あ", "ほ", "ゆ", "英数", "かな"},
	"fr": {"échap", "é", "ù", "◌̂", "alt gr"},
}

// DxRootNodes defines the root nodes for the diagnostics apps across different region codes.
var DxRootNodes = map[string]*nodewith.Finder{
	"us": DxRootNode,
	"jp": nodewith.Name("診断").Role(role.Window),
	"fr": DxRootNode,
}

// DxNarrowMenuButtons defines the hamburger menu button across different region codes.
var DxNarrowMenuButtons = map[string]*nodewith.Finder{
	"us": DxNarrowMenuButton,
	"jp": nodewith.NameContaining("診断").Role(role.Button),
	"fr": DxNarrowMenuButton,
}

// DxInternalKeyboardTestButtons defines test button for internal keyboard which specified by region.
var DxInternalKeyboardTestButtons = map[string]*nodewith.Finder{
	"us": DxInternalKeyboardTestButton,
	"jp": nodewith.NameContaining("デバイス内蔵キーボード").Role(role.Button),
	"fr": nodewith.NameContaining("Tester").Role(role.Button).First(),
}

// DxKeyboardTabButtons defines keyboard tab button which specified by region.
var DxKeyboardTabButtons = map[string]*nodewith.Finder{
	"us": DxKeyboardTab,
	"jp": nodewith.NameContaining("キーボード").Role(role.Button),
	"fr": nodewith.NameContaining("Clavier").Role(role.Button),
}
