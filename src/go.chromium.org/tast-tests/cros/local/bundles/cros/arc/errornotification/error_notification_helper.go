// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package errornotification provides Error Notification helper functions.
package errornotification

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const (
	// Timeout defines default ui timeout.
	Timeout = 30 * time.Second
	// TestAppName is the app name of the test app that can trigger ANR and crash.
	TestAppName = "ArcErrorNotificationTest"
	// TestAppApkName is the app apk name of the test app that can trigger ANR and crash.
	TestAppApkName = TestAppName + ".apk"
	// TestAppPkgName is the pkg name of the test app that can trigger ANR and crash.
	TestAppPkgName = "org.chromium.arc.testapp.errornotificationtest"
	// TestAppActivityName is the main activity name of the test app that can trigger ANR and crash.
	TestAppActivityName = TestAppPkgName + ".MainActivity"
)

// StartActivity starts the main activity in the test app.
func StartActivity(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn) error {
	activity, err := arc.NewActivity(a, TestAppPkgName, TestAppActivityName)
	if err != nil {
		return errors.Wrap(err, "failed to create new activity")
	}
	return activity.Start(ctx, tconn)
}

// ClickButtonWithID clicks the button inside Android app with buttonID.
func ClickButtonWithID(ctx context.Context, device *androidui.Device, buttonID string) error {
	obj := device.Object(androidui.ID(buttonID))
	if err := obj.WaitForExists(ctx, Timeout); err != nil {
		return err
	}
	if err := obj.Click(ctx); err != nil {
		return errors.Wrapf(err, "failed to click %s", buttonID)
	}

	return nil
}

// ClickButtonWithText clicks the button inside buttonText.
func ClickButtonWithText(ctx context.Context, tconn *chrome.TestConn, buttonText string) error {
	ui := uiauto.New(tconn)
	button := nodewith.Name(buttonText).Role(role.StaticText)
	return ui.LeftClick(button)(ctx)
}

// EnsureAppWindowClosed ensures that the test app window is closed.
func EnsureAppWindowClosed(ctx context.Context, tconn *chrome.TestConn) error {
	return ash.WaitForARCAppClosed(ctx, tconn, TestAppPkgName, TestAppName)
}

// MinimizeWindow minimizes the test app window.
func MinimizeWindow(ctx context.Context, tconn *chrome.TestConn) error {
	window, err := ash.GetARCAppWindowInfo(ctx, tconn, TestAppPkgName)
	if err != nil {
		return errors.Wrapf(err, "failed to get ARC window information for package name %s", TestAppPkgName)
	}
	if _, err := ash.SetWindowState(ctx, tconn, window.ID, ash.WMEventMinimize, false /* waitForStateChange */); err != nil {
		return err
	}
	return nil
}
