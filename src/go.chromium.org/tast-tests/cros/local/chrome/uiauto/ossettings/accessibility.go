// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/a11y/pdfocr"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const (
	liveCaptionSubPageURL = "audioAndCaptions"
	liveCaptionToggleName = "Live Caption"
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

// TogglePDFOCR toggles on/off the PDF OCR feature in the Accessibiility page.
func TogglePDFOCR(cr *chrome.Chrome, tconn *chrome.TestConn, value bool) action.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		ttsHeading := nodewith.NameStartingWith("Text-to-Speech").Role(role.Heading)
		settings, err := LaunchAtPageURL(ctx, tconn, cr, pdfocr.SettingsSubPageURL, ui.Exists(ttsHeading))
		if err != nil {
			return errors.Wrap(err, "failed to open Text-to-Speech setting page")
		}
		defer settings.Close(ctx)
		return uiauto.Combine("toggle PDF OCR",
			ui.WaitUntilExists(nodewith.Name(pdfocr.SettingsToggleName).Role(role.ToggleButton)),
			settings.SetToggleOption(cr, pdfocr.SettingsToggleName, value),
		)(ctx)
	}
}
