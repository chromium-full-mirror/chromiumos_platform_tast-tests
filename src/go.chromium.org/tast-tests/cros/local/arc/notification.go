// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
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

const (
	notificationPostPermission = "android.permission.POST_NOTIFICATIONS"
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
	// Unable to programmatically change notification settings before T.
	sdkVer, err := SDKVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get SDK version")
	}
	if sdkVer < SDKT {
		return nil
	}

	if err := a.GrantPermission(ctx, packageName, notificationPostPermission); err != nil {
		return errors.Wrapf(err, "failed to grant %q", notificationPostPermission)
	}

	return nil
}

// DisableAppNotifications disables the notification permission. This
// permanently prevents the app from making a pop-up for notification
// permissions.
func DisableAppNotifications(ctx context.Context, a *ARC, appPkgName string) error {
	// Unable to programmatically block notification settings before T.
	sdkVer, err := SDKVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get SDK version")
	}
	if sdkVer < SDKT {
		return nil
	}

	// The "user-fixed" permission flag permanently prevents the app from asking
	// for notification permissions.
	// See https://developer.android.com/about/versions/11/privacy/permissions#dialog-visibility.
	fixedFlag := "user-fixed"
	if err := a.Command(ctx, "pm", "set-permission-flags", appPkgName, notificationPostPermission, fixedFlag).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to set permission flag %q for permission %q", fixedFlag, notificationPostPermission)
	}
	return nil
}
