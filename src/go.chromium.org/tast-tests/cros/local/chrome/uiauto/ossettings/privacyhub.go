// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const privacyHubPageURL = "osPrivacy/privacyHub"

// LaunchAtPrivacyHubPage launches Settings app on privacy hub page.
func LaunchAtPrivacyHubPage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	ui := uiauto.New(tconn)
	privacyHubPageHeading := nodewith.NameStartingWith("Privacy controls").Role(role.Heading).Ancestor(WindowFinder)
	return LaunchAtPageURL(ctx, tconn, cr, privacyHubPageURL, ui.Exists(privacyHubPageHeading))
}

// ToggleMuteNudge toggles on/off mute nudge option in privacy hub settings.
func ToggleMuteNudge(cr *chrome.Chrome, tconn *chrome.TestConn, value bool) action.Action {
	return ToggleMuteNudgeWithErrorDump(cr, tconn, value, "")
}

// ToggleMuteNudgeWithErrorDump toggles on/off mute nudge option in privacy hub settings.
// It dumps faillog if any error happens.
func ToggleMuteNudgeWithErrorDump(cr *chrome.Chrome, tconn *chrome.TestConn, value bool, path string) action.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		settings, err := LaunchAtPrivacyHubPage(ctx, tconn, cr)
		if err != nil {
			return errors.Wrap(err, "failed to open setting page")
		}
		defer settings.Close(ctx)
		muteNudgeToggleName := "Mute nudge"
		err = uiauto.Combine("toggle mute nudge",
			ui.WaitUntilExists(nodewith.Name(muteNudgeToggleName).Role(role.ToggleButton)),
			settings.SetToggleOption(cr, muteNudgeToggleName, value),
		)(ctx)
		if err != nil && path != "" {
			faillog.DumpUITreeWithScreenshotOnError(ctx, path, func() bool { return true }, cr, "ui_mute_nudge_setting")
		}
		return err
	}
}
