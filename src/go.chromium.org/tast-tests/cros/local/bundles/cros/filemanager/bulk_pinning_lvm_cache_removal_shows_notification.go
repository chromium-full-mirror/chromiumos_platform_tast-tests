// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	dfCommon "go.chromium.org/tast-tests/cros/common/drivefs"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/session"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           BulkPinningLvmCacheRemovalShowsNotification,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verify that a notification is shown when the LVM cache partition is removed",
		BugComponent:   "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
			"fdegros@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_stable",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-c0cadb1a-c202-4e5b-ba26-48993419962b",
		}},
		Timeout: 5 * time.Minute,
	})
}

func BulkPinningLvmCacheRemovalShowsNotification(ctx context.Context, s *testing.State) {
	creds, err := credconfig.PickRandomCreds(dma.CredsFromPool(dfCommon.AccountPoolVarName))
	if err != nil {
		s.Fatal("Failed to pick random credentials to sign in with: ", err)
	}

	// We can't rely on fixtures in this test as the user needs to be logged out
	// and back in, this violates how fixtures work.
	cr, tconn, driveFs, err := createChromeAndDriveFs(ctx, creds, false)
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	// Give enough time for DriveFS logs and screenshot to be taken.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()
	defer driveFs.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := bulkpinning.EnableBulkPinningFromSettings(cr, tconn)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning from settings: ", err)
	}
	defer bulkpinning.DisableBulkPinningFromSettings(cr, tconn)(cleanupCtx)

	// Sign out from the currently signed in user, the existing user needs to be
	// logged back in or else the LVM cache will not be properly identified as
	// having been removed.
	if err := signOut(ctx, tconn); err != nil {
		s.Fatal("Failed to signout of logged in user: ", err)
	}

	// Remove the LVM cache volume when the user is logged out.
	if err := removeLvmCacheVolume(ctx); err != nil {
		s.Fatal("Failed to remove the lvm cache volume: ", err)
	}

	// Create chrome again and reuse the existing session.
	cr, tconn, _, err = createChromeAndDriveFs(ctx, creds, true)
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	// Check the LVM volume removed notification appears.
	if _, err := ash.WaitForNotification(ctx, tconn, 10*time.Second, ash.WaitIDContains("drive_data_dir_missing")); err != nil {
		s.Fatal("Failed to wait for bulk pinning error notification: ", err)
	}
}

// createChromeAndDriveFs creates drivefs and chrome and alternately logs the
// user back into chrome if `reuseSession` is true.
func createChromeAndDriveFs(ctx context.Context, creds credconfig.Creds, reuseSession bool) (*chrome.Chrome, *chrome.TestConn, *drivefs.DriveFs, error) {
	chromeOptions := []chrome.Option{
		chrome.KeepState(),
		chrome.ARCDisabled(),
		chrome.GAIALogin(creds),
		chrome.EnableFeatures("FeatureManagementDriveFsBulkPinning"),
	}
	if reuseSession {
		chromeOptions = append(chromeOptions,
			chrome.TryReuseSession(),
			chrome.RemoveNotification(false),
		)
	}
	cr, err := chrome.New(ctx, chromeOptions...)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create chrome instance")
	}

	// If we're reusing the session, avoid waiting for drive to stabilize as the
	// notification is the only thing we're interested in. This check can
	// sometimes take longer then the notification stays visible on startup.
	var driveFs *drivefs.DriveFs
	if !reuseSession {
		driveFs, err = drivefs.NewDriveFs(ctx, cr.NormalizedUser())
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to create DriveFS")
		}
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create test API")
	}
	return cr, tconn, driveFs, nil
}

// removeLvmCacheVolume tries every second for 10 seconds to remove the LVM
// cache volume until it succeeds or times out. This is necessary as the LVM
// volume can only be removed when not in use (i.e. signed out).
func removeLvmCacheVolume(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := testexec.CommandContext(ctx,
			"bash",
			"-c",
			"lvremove -ff /dev/*/cryptohome-*-cache",
		).Run(); err != nil {
			return errors.Wrap(err, "failed to remove lvm cache")
		}
		testing.ContextLog(ctx, "Successfully removed the lvm cache")
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})
}

// signOut uses quick settings to sign out the current session using `tconn`,
// and wait until the session has actually stopped.
func signOut(ctx context.Context, tconn *chrome.TestConn) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to SessionManager")
	}

	sw, err := sm.WatchSessionStateChanged(ctx, "stopped")
	if err != nil {
		return errors.Wrap(err, "failed to watch for session state change D-Bus signal")
	}
	defer sw.Close(ctx)

	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to sign out with quick settings")
	}

	select {
	case <-sw.Signals:
		testing.ContextLog(ctx, "Session stopped")
	case <-ctx.Done():
		return errors.New("Timed out waiting for session state signal")
	}
	return nil
}
