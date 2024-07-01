// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const (
	liveCaptionSubPageURL          = "audioAndCaptions"
	liveCaptionToggleName          = "Live Caption"
	liveTranslateToggleName        = "Live Translate"
	liveTranslateDropdownName      = "Translate to"
	liveTranslateDropdownClassName = "language-dropdown"
	liveTranslateOptionFinderName  = "Preferred caption language"
)

// ToggleLiveCaption toggles on/off live caption option in Accessibiility tab.
func ToggleLiveCaption(cr *chrome.Chrome, tconn *chrome.TestConn, value bool) action.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		captionsHeading := nodewith.NameStartingWith("Audio and captions").Role(role.Heading)
		settings, err := LaunchAtPageURL(ctx, tconn, cr, liveCaptionSubPageURL, ui.Exists(captionsHeading))
		if err != nil {
			return errors.Wrap(err, "failed to open setting page")
		}
		defer settings.Close(ctx)
		return uiauto.Combine("toggle live caption",
			ui.WaitUntilExists(nodewith.Name(liveCaptionToggleName).Role(role.ToggleButton)),
			settings.SetToggleOption(cr, liveCaptionToggleName, value),
		)(ctx)
	}
}

// ToggleLiveTranslate on or off. Must happen _after_ ToggleLiveCaption has been set to ON.
// set to true/false, and the lang string is only used if value is true.
func ToggleLiveTranslate(cr *chrome.Chrome, tconn *chrome.TestConn, value bool, lang string) action.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		captionsHeading := nodewith.NameStartingWith("Audio and captions").Role(role.Heading)
		settings, err := LaunchAtPageURL(ctx, tconn, cr, liveCaptionSubPageURL, ui.Exists(captionsHeading))
		if err != nil {
			return errors.Wrap(err, "failed to open setting page")
		}
		defer settings.Close(ctx)

		if value {
			return uiauto.Combine("toggle live translate",
				ui.WaitUntilExists(nodewith.Name(liveTranslateToggleName).Role(role.ToggleButton)),
				settings.SetToggleOption(cr, liveTranslateToggleName, value),
				ui.WaitUntilExists(nodewith.Name(liveTranslateOptionFinderName).First()),
				settings.SetDropDownNthOption(cr, liveTranslateOptionFinderName, lang, 1))(ctx)
		}
		return uiauto.Combine("toggle live translate",
			ui.WaitUntilExists(nodewith.Name(liveTranslateToggleName).Role(role.ToggleButton)),
			settings.SetToggleOption(cr, liveTranslateToggleName, value))(ctx)

	}
}
