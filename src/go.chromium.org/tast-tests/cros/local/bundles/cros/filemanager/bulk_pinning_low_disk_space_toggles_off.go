// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningLowDiskSpaceTogglesOff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that bulk pinning turns off when 2GB threshold is breached",
		BugComponent: "b:167289",
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
			Value: "screenplay-9dc60f39-2b08-4e0e-9b7e-fa699612892f",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningLowDiskSpaceTogglesOff(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Give enough time for DriveFS logs and screenshot to be taken.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer fixt.DriveFs.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn)(ctx); err != nil {
		s.Fatal("Failed to toggle bulk pinning: ", err)
	}

	fillFile, err := disk.FillUntil(cleanup.UserHome, cleanup.MinimalFreeSpace)
	if err != nil {
		s.Fatal("Failed to fill disk space: ", err)
	}
	defer func() {
		if err := os.Remove(fillFile); err != nil {
			s.Errorf("Failed to remove fill file %s: %v", fillFile, err)
		}
	}()

	// Check the sync error notification first before the toggle as the
	// notification can disappear, whereas the toggle should stay untoggled once
	// the first error has been received.
	const notificationID = "drive-bulk-pinning-error"
	if _, err := ash.WaitForNotification(ctx, tconn, 10*time.Second, ash.WaitIDContains(notificationID)); err != nil {
		s.Fatal("Failed to wait for bulk pinning error notification: ", err)
	}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(bulkpinning.SettingsToggleFinder.Attribute("checked", "false"))(ctx); err != nil {
		s.Fatal("Failed to wait for the bulk pinning toggle to disable: ", err)
	}
}
