// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RightClickLongPress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks right click is properly converted to long press in compat mode",
		Contacts:     []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,
	})
}

func RightClickLongPress(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	const (
		apk          = "ArcLongPressTest.apk"
		pkg          = "org.chromium.arc.testapp.longpress"
		activityName = ".MainActivity"
	)

	cleanupTabletMode, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to set clamshell mode: ", err)
	}
	defer cleanupTabletMode(cleanupCtx)

	if err := wm.ResetSplashScreenCounter(ctx, tconn); err != nil {
		s.Fatal("Failed to reset splash screen counter: ", err)
	}

	// Uninstall the test app if it's already installed so that per-app settings get cleared.
	installed, err := a.PackageInstalled(ctx, wm.ResizeLockTestPkgName)
	if err != nil {
		s.Fatal("Failed to get package install status: ", err)
	}
	if installed {
		testing.ContextLog(ctx, "The test app is already installed. Trying to uninstall")
		if err := a.Uninstall(ctx, wm.ResizeLockTestPkgName); err != nil {
			s.Fatal("Failed to uninstall app: ", err)
		}
	}
	if err := a.Install(ctx, arc.APKPath(apk), adb.InstallOptionFromPlayStore); err != nil {
		s.Fatal("Failed to install the app: ", err)
	}

	act, err := arc.NewActivity(a, pkg, activityName)
	if err != nil {
		s.Fatalf("Failed to create a new activity %q: %v", activityName, err)
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatalf("Failed to start the activity %q", activityName)
	}

	if err := ash.WaitForVisible(ctx, tconn, act.PackageName()); err != nil {
		s.Fatal("Failed to wait until the activity gets visible: ", err)
	}
	if err := d.WaitForIdle(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for Android to be idle: ", err)
	}

	// Close the compat mode splash dialog.
	if err := wm.CheckVisibility(ctx, tconn, wm.ArcSplashScreenDialogViewClassName, true); err != nil {
		s.Fatal("Failed to wait for splash: ", err)
	}
	if err := wm.CloseSplash(ctx, tconn, wm.InputMethodClick, nil); err != nil {
		s.Fatal("Failed to close splash: ", err)
	}

	window, err := ash.GetARCAppWindowInfo(ctx, tconn, pkg)
	if err != nil {
		s.Fatal("Failed to get ARC app window info: ", err)
	}

	const fieldID = pkg + ":id/long_press_count"

	if err := d.Object(ui.ID(fieldID)).WaitForText(ctx, "0", 30*time.Second); err != nil {
		s.Fatal("Failed to wait for the target view: ", err)
	}

	if err := mouse.Click(tconn, window.TargetBounds.CenterPoint(), mouse.RightButton)(ctx); err != nil {
		s.Fatal("Failed to right click the activity: ", err)
	}

	if err := d.Object(ui.ID(fieldID)).WaitForText(ctx, "1", 30*time.Second); err != nil {
		s.Fatal("Failed to wait for long press: ", err)
	}
}
