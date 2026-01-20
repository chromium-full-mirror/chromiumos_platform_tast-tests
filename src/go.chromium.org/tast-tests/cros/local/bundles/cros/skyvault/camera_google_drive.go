// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           CameraGoogleDrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving camera files to Google Drive when CameraSaveLocation policy is set",
		BugComponent:   "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Contacts: []string{
			"poromov@chromium.org",
			"aidazolic@chromium.org",
			"chromeos-camera-app-eng@google.com",
		},
		SoftwareDeps: []string{
			"camera_app",
			"chrome",
			"chrome_internal",
			"gaia",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DriveDisabled{}, pci.Served),
			pci.SearchFlag(&policy.CameraSaveLocation{}, pci.Served),
		},
		Fixture: "driveFsManagedWithSkyVault",
	})
}

// CameraGoogleDrive tests that photos are saved to Google Drive when forced by policy.
func CameraGoogleDrive(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs
	cameraFolder := fixt.CameraFolder

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "camera_google_drive")
	s.AttachErrorHandlers(handler, handler)

	cr := fixt.Chrome
	outDir := s.OutDir()
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseFakeHALCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge")
	}

	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		s.Fatal("Failed to start CCA: ", err)
	}

	if err := app.AcceptCloudSaveWarning(ctx); err != nil {
		s.Fatal("Failed to accept cloud save warning: ", err)
	}

	if err := app.ClickShutter(ctx); err != nil {
		s.Fatal("Failed to click shutter: ", err)
	}

	// Verify that photo is successfully taken.
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains("skyvault_camera_upload_done_notification"), ash.WaitTitle("Photo saved to Google Drive")); err != nil {
		s.Errorf("Failed to wait for notification with id \"%q\": %v", "skyvault_camera_upload_done_notification", err)
	}

	// Click edit button, check if Gallery app opens up.
	editButton := nodewith.Name("Edit").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to click edit button: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		s.Error("Failed to wait for Gallery app to open: ", err)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	if err := files.OpenDrive()(ctx); err != nil {
		s.Fatal("Failed to open Drive: ", err)
	}

	// Verify that the Camera folder exists.
	if err := files.WaitForFile(cameraFolder)(ctx); err != nil {
		s.Fatal("Camera folder not found on Drive: ", err)
	}
	defer func(ctx context.Context) {
		driveFilePath := dfs.MyDrivePath(cameraFolder)
		if err := os.RemoveAll(driveFilePath); err != nil {
			testing.ContextLog(ctx, "Failed to remove Camera folder: ", err)
		}
	}(cleanupCtx)

	// Open Camera folder.
	if err := files.OpenFile(cameraFolder)(ctx); err != nil {
		s.Fatal("Failed to open Camera folder: ", err)
	}

	// Verify that the photo is saved to Google Drive.
	_, err = files.WaitForFileByPattern(ctx, regexp.MustCompile(`^IMG_.*\.jpg$`))
	if err != nil {
		s.Fatal("Image not found on Drive: ", err)
	}
}
