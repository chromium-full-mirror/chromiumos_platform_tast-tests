// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           CameraOnedrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving camera files to OneDrive when CameraSaveLocation policy is set",
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
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		VarDeps: []string{
			"onedrive.accountPool",
		},
		Fixture: "onedriveManagedWithSkyVaultForCamera",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.CameraSaveLocation{}, pci.Served),
		},
	})
}

// CameraOnedrive tests that photos are saved to OneDrive when forced by policy.
func CameraOnedrive(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	cameraFolder := data.CameraFolder

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "camera_onedrive")
	s.AttachErrorHandlers(handler, handler)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	// Connect to OneDrive.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := office.ConnectToOneDrive(cr, tconn, files, ms365App)(ctx); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

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
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains("skyvault_camera_upload_done_notification"), ash.WaitTitle("Photo saved to OneDrive")); err != nil {
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

	files, err = filesapp.Relaunch(ctx, tconn, files)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Verify that the Camera folder exists.
	if err := files.WaitForFile(cameraFolder)(ctx); err != nil {
		s.Fatal("Camera folder not found on OneDrive: ", err)
	}
	defer func(ctx context.Context) {
		if err := files.OpenOneDrive()(ctx); err != nil {
			s.Fatal("Failed to open OneDrive: ", err)
		}
		files.DeleteFileOrFolder(kb, cameraFolder)(ctx)
	}(cleanupCtx)

	// Open Camera folder.
	if err := files.OpenFile(cameraFolder)(ctx); err != nil {
		s.Fatal("Failed to open Camera folder: ", err)
	}

	// Verify that the photo is saved to OneDrive.
	_, err = files.WaitForFileByPattern(ctx, regexp.MustCompile(`^IMG_.*\.jpg$`))
	if err != nil {
		s.Fatal("Image not found on OneDrive: ", err)
	}
}
