// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package browserui contains the general browser UI finders and functions.
package browserui

import (
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// AddressBarFinder represents the address bar node finder.
var AddressBarFinder = nodewith.HasClass("OmniboxViewViews").Role(role.TextField)

// ExtensionsToolbarButton represents the extensions button in browser bar.
var ExtensionsToolbarButton = nodewith.Name("Extensions").Role(role.PopUpButton).HasClass("ExtensionsToolbarButton")

// ExtensionPopupRootView represents the extension popup root window.
var ExtensionPopupRootView = nodewith.Role(role.WebView).HasClass("ExtensionViewViews").First()

var (
	newTabNameRegex      = regexp.MustCompile(`(?i)^New Tab$`)
	newTabClassNameRegex = regexp.MustCompile(`(ToolbarButton|TabStripControlButton)`)
	// NewTabButton represents the "+" button in tab list to create a new tab page
	// which matches the "New Tab" button in the UI, handling both variations:
	// 1. "New tab" with class "ToolbarButton".
	// 2. "New Tab" with class "TabStripControlButton".
	NewTabButton = nodewith.NameRegex(newTabNameRegex).Role(role.Button).ClassNameRegex(newTabClassNameRegex)
)
