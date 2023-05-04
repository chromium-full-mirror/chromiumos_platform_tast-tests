// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package devicesettings contains utilities for working with the device settings
// page.
package devicesettings

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/errors"
)

// Remap changes the action performed by a modifier key.
func Remap(ctx context.Context, ui *uiauto.Context, from, to string) error {
	keyRow := nodewith.Name(from).Role(role.GenericContainer)
	key := nodewith.Name(from).Role(
		role.ComboBoxSelect).Ancestor(keyRow)
	option := nodewith.Name(to).Role(role.ListBoxOption)
	if err := uiauto.Combine(fmt.Sprintf("choose %q option", to),
		ui.LeftClickUntil(key, ui.WithTimeout(
			2*time.Second).WaitUntilExists(option)),
		ui.LeftClick(option),
		ui.WaitUntilExists(option),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to choose %q option", to)
	}
	return nil
}
