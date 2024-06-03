// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package expandable provides utilities to interact with expandable sections.
package expandable

import (
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
)

// EnsureExpandableSectionOpened returns an action to retry to expand the section until it is expanded.
func EnsureExpandableSectionOpened(tconn *chrome.TestConn, section *nodewith.Finder) uiauto.Action {
	ui := uiauto.New(tconn)
	return uiauto.Combine("expand the section",
		ui.WaitUntilExists(section),
		ui.MakeVisible(section),
		ui.RetryUntil(
			ui.LeftClick(section),
			ui.WithTimeout(3*time.Second).WaitUntilExists(section.Expanded()),
		),
	)
}
