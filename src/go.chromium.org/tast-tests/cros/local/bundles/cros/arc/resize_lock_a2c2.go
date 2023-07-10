// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	configNamespace          = "arc_app_compat"
	o4cRecentAddedFlagName   = "o4c_recent_added"
	o4cRecentRemovedFlagName = "o4c_recent_removed"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ResizeLockA2C2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that ARC++ Resize Lock via A2C2 works as expected",
		Contacts:     []string{"arc-framework+tast@google.com", "toshikikikuchi@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Data:         []string{wm.WhiteWallpaperFileName},
		Timeout:      5 * time.Minute,
	})
}

func ensureOverridePhenotypeFlag(ctx context.Context, a *arc.ARC) error {
	if err := a.Command(ctx, "device_config", "put", configNamespace, o4cRecentAddedFlagName, wm.ResizeLockO4CViaA2C2PkgName).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to override phenotype flag for %s/%s", configNamespace, o4cRecentAddedFlagName)
	}
	if err := a.Command(ctx, "device_config", "put", configNamespace, o4cRecentRemovedFlagName, wm.ResizeLockNonO4CViaA2C2PkgName).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to override phenotype flag for %s/%s", configNamespace, o4cRecentRemovedFlagName)
	}
	return nil
}

func testResizeLockState(ctx, cleanupCtx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, cr *chrome.Chrome, isNonO4CViaA2C2 bool) error {
	pkgName := wm.ResizeLockO4CViaA2C2PkgName
	if isNonO4CViaA2C2 {
		pkgName = wm.ResizeLockNonO4CViaA2C2PkgName
	}
	activityName := wm.ResizeLockMainActivityName

	// GMS may periodically sync the flags so we need to override them every time before we start the activity.
	if err := ensureOverridePhenotypeFlag(ctx, a); err != nil {
		return errors.Wrap(err, "failed to ensure to override the phenotype flag")
	}

	// Launch the test app.
	activity, err := arc.NewActivity(a, pkgName, activityName)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", activityName)
	}
	defer activity.Close(ctx)
	if err := activity.Start(ctx, tconn); err != nil {
		return errors.Wrapf(err, "failed to start %s", activityName)
	}
	defer activity.Stop(cleanupCtx, tconn)
	if err := ash.WaitForVisible(ctx, tconn, activity.PackageName()); err != nil {
		return errors.Wrap(err, "failed to wait until the activity gets visible")
	}

	// Verify the resize lock state.
	expectedMode := wm.NoneResizeLockMode
	isSplashVisible := false
	if isNonO4CViaA2C2 {
		expectedMode = wm.PhoneResizeLockMode
		isSplashVisible = true
	}
	if err := wm.CheckResizeLockState(ctx, tconn, cr, activity, expectedMode, isSplashVisible); err != nil {
		return errors.Wrapf(err, "failed to verify the resize lock state of %s/%s", pkgName, activityName)
	}

	return nil
}

func ResizeLockA2C2(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	cleanupTabletMode, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanupTabletMode(cleanupCtx)

	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}

	cleanupShelfAlignment, err := ash.EnsureShelfAlignmentBottom(ctx, tconn, dispInfo.ID)
	if err != nil {
		s.Fatal("Failed to ensure shelf alignment is Bottom: ", err)
	}
	// Be nice and restore shelf alignment to its original state on exit.
	defer cleanupShelfAlignment(cleanupCtx)

	cleanupShelfBehavior, err := ash.EnsureShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorNeverAutoHide)
	if err != nil {
		s.Fatal("Failed to ensure shelf behavior is Never Auto Hide: ", err)
	}
	// Be nice and restore shelf behavior to its original state on exit.
	defer cleanupShelfBehavior(cleanupCtx)

	// Set a pure white wallpaper to reduce the noises on a screenshot because currently checking the visibility of the translucent window border relies on a screenshot.
	// The wallpaper will exist continuous if the Chrome session gets reused.
	if err := wm.SetSolidWhiteWallpaper(ctx, tconn, s); err != nil {
		s.Fatal("Failed to set the white wallpaper: ", err)
	}

	if err := wm.ResetSplashScreenCounter(ctx, tconn); err != nil {
		s.Fatal("Failed to reset splash screen count: ", err)
	}

	// Install the test apps.
	for _, app := range []struct {
		apkName string
		pkgName string
	}{
		{wm.ResizeLockO4CViaA2C2ApkName, wm.ResizeLockO4CViaA2C2PkgName},
		{wm.ResizeLockNonO4CViaA2C2ApkName, wm.ResizeLockNonO4CViaA2C2PkgName},
	} {
		if err := a.Install(ctx, arc.APKPath(app.apkName), adb.InstallOptionFromPlayStore); err != nil {
			s.Fatal("Failed to install app from PlayStore: ", err)
		}
		defer a.Uninstall(cleanupCtx, app.pkgName)
	}

	if err := testResizeLockState(ctx, cleanupCtx, tconn, a, d, cr, true /* isNonO4CViaA2C2 */); err != nil {
		s.Fatal("Failed to verify the resize lock state of the app declared as non-O4C via A2C2: ", err)
	}
	if err := testResizeLockState(ctx, cleanupCtx, tconn, a, d, cr, false /* isNonO4CViaA2C2 */); err != nil {
		s.Fatal("Failed to verify the resize lock state of the app declared as O4C via A2C2: ", err)
	}
}
