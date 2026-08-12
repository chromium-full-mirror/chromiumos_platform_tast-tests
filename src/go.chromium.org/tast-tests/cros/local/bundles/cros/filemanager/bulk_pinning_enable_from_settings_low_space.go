// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           BulkPinningEnableFromSettingsLowSpace,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that the bulk pinning can't be enabled from settings with low disk space",
		BugComponent:   "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
			"gaia",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable",
		},
		Data: []string{
			"test_1KB.txt",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-008c8610-3a45-4adc-a744-aaa57a3683de",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningEnableFromSettingsLowSpace(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	driveFsClient := fixt.DriveFs

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Reserve enough time for the fill file to be cleaned up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	diskCleanup, err := bulkpinning.FillDiskUntilNotEnoughSpace(ctx)
	if err != nil {
		s.Fatal("Failed to fill the users cryptohome to minimal free space: ", err)
	}
	defer diskCleanup()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, fixt.Chrome, bulkpinning.GoogleDriveSettingsPageURL, ui.Exists(bulkpinning.SettingsToggleFinder)); err != nil {
		s.Fatal("Failed to launch settings page: ", err)
	}

	notEnoughSpaceDialog := nodewith.Role(role.Dialog).Name("Not enough storage space")
	if err := uiauto.Combine("toggle bulk pinning",
		// Ensure the toggle is disabled to begin with.
		ui.Exists(bulkpinning.SettingsToggleFinder.Attribute("checked", "false")),
		// Toggle the bulk pinning toggle in OS Settings and wait until the
		// "Not enough storage space" dialog appears.
		ui.LeftClickUntil(
			bulkpinning.SettingsToggleFinder,
			ui.WithTimeout(2*time.Second).WaitUntilExists(notEnoughSpaceDialog),
		),
		// Keep clicking the OK button until the dialog disappears.
		ui.LeftClickUntil(
			nodewith.Role(role.Button).Name("OK"),
			ui.WithTimeout(2*time.Second).WaitUntilGone(notEnoughSpaceDialog),
		),
		// Assert the toggle is still in the unchecked state.
		ui.Exists(bulkpinning.SettingsToggleFinder.Attribute("checked", "false")),
	)(ctx); err != nil {
		s.Fatal("Failed to toggle bulk pinning: ", err)
	}
}
