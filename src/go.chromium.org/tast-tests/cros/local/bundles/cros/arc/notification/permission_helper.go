// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package notification provides Notification Helper functions.
package notification

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
)

// EnsureNotificationPermission grants the notification permission if needed.
func EnsureNotificationPermission(ctx context.Context, a *arc.ARC, packageName string) error {
	const (
		notificationPostPermission = "android.permission.POST_NOTIFICATIONS"
	)
	// Starts from T, notification permission must be granted explicitly.
	sdkVer, err := arc.SDKVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get SDKVersion")
	}
	if sdkVer >= arc.SDKT {
		if err := a.GrantPermission(ctx, packageName, notificationPostPermission); err != nil {
			return errors.Wrapf(err, "failed to grant %s", notificationPostPermission)
		}
	}

	return nil
}
