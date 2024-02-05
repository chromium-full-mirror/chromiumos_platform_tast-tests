// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
)

var (
	// ArcNotificationContentView is the class name of ArcNotificationContentView.
	ArcNotificationContentView = nodewith.HasClass("ArcNotificationContentView")
)

// ClickNotificationExpandButtonAndWaitForAnimation clicks the expand button and waits for its animation.
func ClickNotificationExpandButtonAndWaitForAnimation(ctx context.Context, ui *uiauto.Context, mousePC pointer.Context) error {
	bounds, err := ui.Location(ctx, ArcNotificationContentView)
	if err != nil {
		return errors.Wrap(err, "failed to get the notification bounds")
	}

	// TODO(b/224685870): Use UIAutomator to get the button position dynamically when it supports multi-display.
	expandButtonOffset := coords.NewPoint(-28, 44)
	if err := mousePC.ClickAt(bounds.TopRight().Add(expandButtonOffset))(ctx); err != nil {
		return errors.Wrap(err, "failed to click the notification")
	}

	// Wait until animation finished.
	if err := ui.WithInterval(2 * time.Second).WaitForLocation(ArcNotificationContentView)(ctx); err != nil {
		return errors.Wrap(err, "the notification did not stop animating")
	}

	return nil
}

// EnsureNotificationPermission grants the notification permission if needed.
func EnsureNotificationPermission(ctx context.Context, a *ARC, packageName string) error {
	const (
		notificationPostPermission = "android.permission.POST_NOTIFICATIONS"
	)
	// Starts from T, notification permission must be granted explicitly.
	sdkVer, err := SDKVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get SDKVersion")
	}
	if sdkVer >= SDKT {
		if err := a.GrantPermission(ctx, packageName, notificationPostPermission); err != nil {
			return errors.Wrapf(err, "failed to grant %s", notificationPostPermission)
		}
	}

	return nil
}
