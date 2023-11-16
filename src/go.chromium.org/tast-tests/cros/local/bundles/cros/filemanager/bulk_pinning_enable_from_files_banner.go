// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningEnableFromFilesBanner,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the bulk pinning feature can be enabled via the banner in Files app",
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
		Data: []string{
			"test_1KB.txt",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-7d6845fa-906f-4322-ba2d-16e1f56b7571",
		}},
		Timeout: 5 * time.Minute,
		Fixture: "driveFsStartedBulkPinningEnabled",
	})
}

func BulkPinningEnableFromFilesBanner(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	apiClient := fixt.APIClient
	driveFsClient := fixt.DriveFs

	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to open the Test API connection: ", err)
	}

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	// Upload a file to Drive via the Drive API. This file will not be locally
	// cached thus enabling bulk pinning should cause it to be made available
	// offline.
	fileName := filemanager.GenerateTestFileName(s.TestName() + ".txt")
	fileID, err := bulkpinning.CreateTestFile(ctx, apiClient, driveFsClient, s.DataPath("test_1KB.txt"), fileName)
	if err != nil {
		s.Fatal("Failed to create a test file: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, fileID); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	bulkPinningBanner := nodewith.Role(role.Banner).ClassName("tast-bulk-pinning-banner")
	dismissButton := nodewith.Role(role.Button).Name("Dismiss")
	getStartedButton := nodewith.Role(role.Button).Name("Get started")
	turnOnButton := nodewith.Role(role.Button).Name("Turn on")
	if err := uiauto.Combine("open drive and enable bulk pinning",
		files.OpenDrive(),
		files.LeftClickUntil(dismissButton, files.WithTimeout(2*time.Second).WaitUntilExists(bulkPinningBanner)),
		files.LeftClickUntil(getStartedButton, files.WaitUntilExists(turnOnButton)),
		files.LeftClickUntil(turnOnButton, files.WaitUntilGone(nodewith.Role(role.Dialog))),
	)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning from Files: ", err)
	}

	tableRow := nodewith.Role(role.ListBoxOption)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		tableRows, err := files.NodesInfo(ctx, tableRow)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get node info"))
		}
		if len(tableRows) < 1 {
			return errors.New("still waiting for at least one table row to appear")
		}
		for _, row := range tableRows {
			// Keep trying until all files contain "Available offline". This excludes:
			//  - Folders: They don't get an available offline tick.
			//  - Hosted files: These take much longer to appear with a tick and are
			//    covered in other tests, so consider only blobs with a tick as
			//    successful.
			if !strings.Contains(row.Name, "Available offline") &&
				!strings.Contains(row.Name, "Type Folder") &&
				!strings.Contains(row.Name, "Type Google") {
				return errors.Errorf("still waiting for all files to be available offline %q", row.Name)
			}
		}
		return nil
	}, &testing.PollOptions{}); err != nil {
		s.Fatal("Failed to wait for all files to have 'Available offline' status: ", err)
	}
}
