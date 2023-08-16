// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsDssOffline,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify that making a Docs/Sheets/Slides file available offline through Files App works",
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
			Val:     browser.TypeAsh,
			Fixture: "driveFsStartedWithNativeMessaging",
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "driveFsStartedWithNativeMessagingLacros",
		}},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-32d74807-7b2f-46ae-8bef-b34b76ab328c",
			},
		},
	})
}

func DrivefsDssOffline(ctx context.Context, s *testing.State) {
	APIClient := s.FixtValue().(*drivefs.FixtureData).APIClient
	tconn := s.FixtValue().(*drivefs.FixtureData).TestAPIConn
	driveFsClient := s.FixtValue().(*drivefs.FixtureData).DriveFs

	uniqueSuffix := fmt.Sprintf("-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	testDocFileName := fmt.Sprintf("doc-drivefs%s", uniqueSuffix)
	uniqueTestFolderName := fmt.Sprintf("DrivefsDssOffline%s", uniqueSuffix)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	br, closeBrowser, err := browserfixt.SetUp(ctx, s.FixtValue().(*drivefs.FixtureData).Chrome, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Create the unique folder that will be directly navigated to below.
	testFilePath := driveFsClient.MyDrivePath(uniqueTestFolderName, testDocFileName)
	folder, err := APIClient.Createfolder(ctx, uniqueTestFolderName, []string{"root"})
	if err != nil {
		s.Fatal("Failed to create folder in MyDrive: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, folder.Id)

	// Create a blank Google doc in the nested folder above.
	file, err := APIClient.CreateBlankGoogleDoc(ctx, testDocFileName, []string{folder.Id})
	if err != nil {
		s.Fatal("Failed to create blank google doc: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, file.Id)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)

	if err := helpers.InstallRequiredExtensions(ctx, br, tconn); err != nil {
		s.Fatal("Failed to install the required extensions: ", err)
	}

	var filesApp *filesapp.FilesApp
	testFileNameWithExt := fmt.Sprintf("%s.gdoc", testDocFileName)
	// There is a small period of time on startup where DriveFS can't pin Docs files, so repeatedly
	// relaunch Files App until the Available offline toggle is enabled.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Launch Files App directly to the directory containing the `testFilePath`.
		filesApp, err = filesapp.LaunchSWAToPath(ctx, tconn, filepath.Dir(testFilePath))
		if err != nil {
			return err
		}
		filesApp.WaitForFile(testFileNameWithExt)(ctx)
		filesApp.SelectFile(testFileNameWithExt)(ctx)
		nodeInfo, err := filesApp.Info(ctx, nodewith.Name("Available offline").Role(role.ToggleButton))
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the current status of the Available offline toggle"))
		}
		if _, disabled := nodeInfo.HTMLAttributes["disabled"]; disabled {
			filesApp.Close(ctx)
			return errors.New("the Available offline toggle is still disabled")
		}
		return nil
	}, &testing.PollOptions{Interval: 500 * time.Millisecond}); err != nil {
		s.Fatal("Failed to wait for the Available offline toggle to be enabled: ", err)
	}

	// Try make the newly created Google doc available offline.
	if err := filesApp.ToggleAvailableOfflineForFile(testFileNameWithExt)(ctx); err != nil {
		s.Fatalf("Failed to make test file %q in Drive available offline: %v", testFileNameWithExt, err)
	}

	// Sometimes offline is already enabled and the enable offline notification does not show,
	// so add a timeout when trying to dismiss the notification.
	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(2 * time.Second).LeftClick(nodewith.Name("Enable Offline").Role(role.Button))(ctx); err != nil {
		s.Log("Failed to enable offline, offline might already be enabled")
	}

	s.Log("Waiting for the Available offline toggle to stabilize")
	var previousNodeInfo *uiauto.NodeInfo
	var currentNodeInfo *uiauto.NodeInfo
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		currentNodeInfo, err = filesApp.Info(ctx, nodewith.Name("Available offline").Role(role.ToggleButton))
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get info of the Available offline toggle"))
		}
		if previousNodeInfo != nil && previousNodeInfo.Checked == currentNodeInfo.Checked {
			return nil
		}
		previousNodeInfo = currentNodeInfo
		return errors.New("the Available offline toggle did not stabilize")
	}, &testing.PollOptions{Interval: 500 * time.Millisecond}); err != nil {
		s.Fatal("Failed to wait for the Available offline toggle to stabilize: ", err)
	}

	if currentNodeInfo.Checked != checked.True {
		s.Fatalf("The test file %q in Drive was not made available offline", testFileNameWithExt)
	}
}
