// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mapui

import "go.chromium.org/tast/core/testing"

var osSettingsAddGoogleAccountButton = testing.RegisterVarString(
	"mapui.ossettings.add_google_account_button",
	`{"name": "Add Google Account", "role": "button"}`,
	"UI Node for the Add Google Account button in the OS settings",
)

// OSSettingsAddGoogleAccountButton is a node finder for the Add Google Account button in the OS settings.
var OSSettingsAddGoogleAccountButton = nodeFromStringVar(osSettingsAddGoogleAccountButton)
