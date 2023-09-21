// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package imesettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

const inputSubpageURL = "osLanguages/input"

// LaunchAtSuggestionSettingsPage launches the Input subpage of the Settings app
// which contains suggestions settings.
func LaunchAtSuggestionSettingsPage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*IMESettings, error) {
	ui := uiauto.New(tconn)
	suggestionPageHeading := nodewith.NameStartingWith("Suggestions").Role(role.Heading).Ancestor(ossettings.WindowFinder)
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, inputSubpageURL, ui.Exists(suggestionPageHeading))
	if err != nil {
		return nil, err
	}
	return &IMESettings{settings.WithPollOpts(defaultPollOpts)}, nil
}
