// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lockscreen

import (
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// HidePasswordButton is the finder for the "Hide password" button on Lock/Start screen.
var HidePasswordButton = nodewith.Role(role.ToggleButton).HasClass("DisplayPasswordButton").Name("Hide password")

// ShowPasswordButton is the finder for the "Show password" button on Lock/Start screen.
var ShowPasswordButton = nodewith.Role(role.Button).HasClass("DisplayPasswordButton").Name("Show password")

// SubmitButton is the finder for the "Submit" button on Lock/Start screen.
var SubmitButton = nodewith.Name("Submit").Role(role.Button)

// SwitchToPasswordButton is the finder for the "Switch to password" button on Lock/Start screen.
var SwitchToPasswordButton = nodewith.Role(role.Button).Name("Switch to password")
