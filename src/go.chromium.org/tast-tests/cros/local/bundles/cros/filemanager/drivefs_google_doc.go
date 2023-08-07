// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsGoogleDoc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that a google doc created via Drive API syncs to DriveFS",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"austinct@chromium.org",
			"benreich@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		Timeout: 5 * time.Minute,
		Params: []testing.Param{{
			Fixture: "driveFsStarted",
		}, {
			Name:    "chrome_networking",
			Fixture: "driveFsStartedWithChromeNetworking",
		}},
	})
}

func DrivefsGoogleDoc(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	APIClient := fixt.APIClient
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Current refresh period is 2 minutes, leaving buffer for UI propagation.
	// TODO(crbug/1112246): Reduce refresh period once push notifications fixed.
	const filesAppUITimeout = 3 * time.Minute
	testDocFileName := fmt.Sprintf("doc-drivefs-%d-%d", time.Now().UnixNano(), rand.Intn(10000))

	// Create a blank Google doc in the root GDrive directory.
	file, err := APIClient.CreateBlankGoogleDoc(ctx, testDocFileName, []string{"root"})
	if err != nil {
		s.Fatal("Could not create blank google doc: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, file.Id)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	// Launch Files App and check that Drive is accessible.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Could not launch the Files App: ", err)
	}

	// Navigate to Google Drive via the Files App ui.
	if err := filesApp.OpenDrive()(ctx); err != nil {
		s.Fatal("Could not open Google Drive folder: ", err)
	}

	// Check for the test file created earlier.
	testFileNameWithExt := fmt.Sprintf("%s.gdoc", testDocFileName)
	if err := filesApp.WithTimeout(filesAppUITimeout).WaitForFile(testFileNameWithExt)(ctx); err != nil {
		s.Fatalf("Could not find the test file %q in Drive: %v", testFileNameWithExt, err)
	}
}
