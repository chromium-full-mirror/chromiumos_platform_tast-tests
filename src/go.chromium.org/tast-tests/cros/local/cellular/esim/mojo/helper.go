// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mojo

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// Manager creates a connection to the esim_manager Mojo API.
func Manager(ctx context.Context, cr *chrome.Chrome, slot int) (*ESimManager, error) {
	conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/internet")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open settings app")
	}

	var js chrome.JSObject

	if err := conn.Call(ctx, &js, ESimManagerJS); err != nil {
		return nil, errors.Wrap(err, "failed to create eSIM mojo JS object")
	}

	manager := &ESimManager{JS: &js}

	return manager, nil
}
