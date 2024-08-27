// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/errornotification"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ErrorNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks error notification works for arc as expected",
		Contacts:     []string{"arc-framework+tast@google.com", "yaoqq@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm_t"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,
	})
}

func ErrorNotification(ctx context.Context, s *testing.State) {
	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	device := d.UIDevice
	cr := d.Chrome

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(errornotification.TestAppApkName)); err != nil {
		s.Fatal("Failed to install the app: ", err)
	}
	defer a.Uninstall(cleanupCtx, errornotification.TestAppPkgName)

	if err := testCrash(ctx, a, tconn, device); err != nil {
		s.Error("Failed to run test for crash: ", err)
	}

	if err := testANR(ctx, a, tconn, device); err != nil {
		s.Error("Failed to run test for ANR: ", err)
	}
}

func testCrash(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn, device *androidui.Device) error {
	const (
		crashButtonID          = errornotification.TestAppPkgName + ":id/crashButton"
		crashNotificationTitle = errornotification.TestAppName + " keeps stopping"
		appInfoHeaderID        = "com.android.settings:id/entity_header_title"
	)
	testing.ContextLog(ctx, "Testing background crash can trigger Chrome notification")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*5)
	defer cancel()

	if err := errornotification.StartActivity(ctx, a, tconn); err != nil {
		return errors.Wrap(err, "failed to start activity")
	}
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	// Trigger first crash (Android defaults to not notify user for the first crash).
	if err := errornotification.ClickButtonWithID(ctx, device, crashButtonID); err != nil {
		return errors.Wrap(err, "failed to click crash button to trigger the first crash")
	}

	// Make sure app window is closed.
	if err := errornotification.EnsureAppWindowClosed(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to verify window closure after first crash")
	}

	if err := errornotification.StartActivity(ctx, a, tconn); err != nil {
		return errors.Wrap(err, "failed to start activity")
	}

	// Trigger second crash (this time Android will notify user).
	if err := errornotification.ClickButtonWithID(ctx, device, crashButtonID); err != nil {
		return errors.Wrap(err, "failed to click crash button to trigger the second crash")
	}

	// Minimize the window to make it into background.
	if err := errornotification.MinimizeWindow(ctx, tconn); err != nil {
		return errors.Wrapf(err, "failed to minimize %s", errornotification.TestAppActivityName)
	}

	// Make sure Chrome notification is shown.
	if _, err := ash.WaitForNotification(ctx, tconn, errornotification.Timeout, ash.WaitTitle(crashNotificationTitle)); err != nil {
		return errors.Wrapf(err, "failed to verify expected notification %s shown", crashNotificationTitle)
	}

	// Click "App info" button on the notification.
	if err := errornotification.ClickButtonWithText(ctx, tconn, "App info"); err != nil {
		return errors.Wrap(err, "failed to click App info button for crash notification")
	}

	// Make sure App info is shown.
	obj := device.Object(androidui.ID(appInfoHeaderID))
	if err := obj.WaitForExists(ctx, errornotification.Timeout); err != nil {
		return errors.Wrap(err, "failed to check setting app info")
	}
	text, err := obj.GetText(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get text from setting app info header")
	}
	if text != errornotification.TestAppName {
		return errors.Errorf("found app info for %s, expected %s", text, errornotification.TestAppName)
	}

	return nil
}

func testANR(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn, device *androidui.Device) error {
	const (
		anrButtonID          = errornotification.TestAppPkgName + ":id/anrButton"
		anrNotificationTitle = errornotification.TestAppName + " isn't responding"
	)
	testing.ContextLog(ctx, "Testing background ANR can trigger Chrome notification")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*5)
	defer cancel()

	if err := errornotification.StartActivity(ctx, a, tconn); err != nil {
		return errors.Wrap(err, "failed to start activity")
	}
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	// Click the button to trigger sleep in the main thread of the app.
	if err := errornotification.ClickButtonWithID(ctx, device, anrButtonID); err != nil {
		return errors.Wrap(err, "failed to click anr button to trigger sleep")
	}

	// Minimize the window to make it into background.
	if err := errornotification.MinimizeWindow(ctx, tconn); err != nil {
		return errors.Wrapf(err, "failed to minimize %s", errornotification.TestAppActivityName)
	}

	// Make sure Chrome notification is shown.
	if _, err := ash.WaitForNotification(ctx, tconn, errornotification.Timeout, ash.WaitTitle(anrNotificationTitle)); err != nil {
		return errors.Wrapf(err, "failed to verify expected notification %s shown", anrNotificationTitle)
	}

	// Click "Close" button on the notification.
	if err := errornotification.ClickButtonWithText(ctx, tconn, "Close"); err != nil {
		return errors.Wrap(err, "failed to click Close button on ANR notification")
	}

	if err := errornotification.EnsureAppWindowClosed(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to verify window closure after clicking Close button on ANR notification")
	}

	return nil
}
