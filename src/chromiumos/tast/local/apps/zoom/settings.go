// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
)

var (
	settingsDialog    = nodewith.Name("settings dialog window").Role(role.Application).Ancestor(zoomMainWebArea)
	moreOptionsButton = nodewith.Name("More meeting control").Ancestor(zoomMainWebArea)
	closeDialogButton = nodewith.Role(role.Button).HasClass("settings-dialog__close").Ancestor(settingsDialog)

	backgroundTab = nodewith.Name("Background").Role(role.Tab).Ancestor(settingsDialog)
)

// SetJoinAudio chooses an audio option or dismisses the dialog on the Join Audio page.
func (zm *Zoom) SetJoinAudio(expectedValue, isDialogAlreadyOpened bool) action.Action {
	joinAudioByComputerButton := nodewith.Name("Join Audio by Computer").Role(role.Button)
	closeButton := nodewith.Name("close").HasClass("join-dialog__close").Role(role.Button)
	joinAudioButton := nodewith.Name("Join Audio").Role(role.Button).Focusable()
	ui := zm.ui

	return func(ctx context.Context) error {
		if !expectedValue && !isDialogAlreadyOpened {
			return nil
		}

		if isDialogAlreadyOpened {
			if err := ui.WaitUntilExists(closeButton)(ctx); err != nil {
				return errors.Wrap(err, "join audio dialog is not found")
			}
			if !expectedValue {
				return ui.DoDefaultUntil(
					closeButton,
					ui.WithTimeout(shortUITimeout).WaitUntilGone(closeButton))(ctx)
			}
		}

		return uiauto.Combine("join audio by computer",
			ui.DoDefault(joinAudioButton),
			ui.DoDefault(joinAudioByComputerButton),
			ui.WaitUntilGone(closeButton),
		)(ctx)
	}
}
