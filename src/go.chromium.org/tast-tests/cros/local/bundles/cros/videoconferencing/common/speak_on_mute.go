// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// SpeakOnMuteNudge matches the UI node for the speak on mute nudge.
var SpeakOnMuteNudge = nodewith.NameStartingWith("Are you talking?").Role(role.StaticText).First()
