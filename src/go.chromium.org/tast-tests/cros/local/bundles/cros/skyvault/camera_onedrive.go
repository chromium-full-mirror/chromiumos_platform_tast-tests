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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		Timeout: 4 * time.Minute,
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
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "camera_onedrive")
	s.AttachErrorHandlers(handler, handler)

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
	onedrive.DeleteDirsInRootByPrefixes(ctx, "Camera_", "ChromeOS device")

	outDir := s.OutDir()
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseFakeHALCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(cleanupCtx)

	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		s.Fatal("Failed to start CCA: ", err)
	}
	defer app.Close(cleanupCtx)

	if err := app.AcceptCloudSaveWarning(ctx); err != nil {
		s.Fatal("Failed to accept cloud save warning: ", err)
	}

	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		testing.ContextLog(ctx, "Failed to close existing notifications before taking photo: ", err)
	}

	defer func(ctx context.Context) {
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			testing.ContextLog(ctx, "Failed to close notifications: ", err)
		}
		onedrive.DeleteDirsInRootByPrefixes(ctx, cameraFolder)
	}(cleanupCtx)

	if err := app.ClickShutter(ctx); err != nil {
		s.Fatal("Failed to click shutter: ", err)
	}

	// Verify that photo is successfully taken.
	if _, err := ash.WaitForNotification(ctx, tconn, time.Minute, ash.WaitIDContains("skyvault_camera_upload_done_notification"), ash.WaitTitle("Photo saved to OneDrive")); err != nil {
		s.Errorf("Failed to wait for notification with id \"%q\": %v", "skyvault_camera_upload_done_notification", err)
	}

	// Click edit button, check if Gallery app opens up.
	if err := clickCameraNotificationEditAndWaitForGallery(ctx, tconn); err != nil {
		s.Fatal("Failed to open photo in Gallery from notification: ", err)
	}
	if err := apps.Close(ctx, tconn, apps.Gallery.ID); err != nil {
		testing.ContextLog(ctx, "Failed to close Gallery app: ", err)
	} else if err := ash.WaitForAppClosed(ctx, tconn, apps.Gallery.ID); err != nil {
		testing.ContextLog(ctx, "Failed to wait for Gallery app to close: ", err)
	}

	files, err = filesapp.Relaunch(ctx, tconn, files)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	if err := onedrive.OpenOneDrive(files)(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Verify that the Camera folder exists.
	if err := onedrive.WaitForFileByPattern(ctx, files, regexp.MustCompile("^"+regexp.QuoteMeta(cameraFolder)+"$")); err != nil {
		s.Fatal("Camera folder not found on OneDrive: ", err)
	}

	// Open Camera folder.
	if err := files.OpenFile(cameraFolder)(ctx); err != nil {
		s.Fatal("Failed to open Camera folder: ", err)
	}

	// Verify that the photo is saved to OneDrive.
	if _, err := files.WaitForFileByPattern(ctx, regexp.MustCompile(`^IMG_.*\.jpg$`)); err != nil {
		s.Fatal("Image not found on OneDrive: ", err)
	}
}

func clickCameraNotificationEditAndWaitForGallery(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	editButton := nodewith.NameRegex(regexp.MustCompile("(?i)^edit$")).Role(role.Button).First()
	notificationTray := nodewith.HasClass("NotificationCenterTray").Role(role.Button).First()
	expandButton := nodewith.HasClass("AshNotificationExpandButton").First()

	clickEdit := func(ctx context.Context) error {
		if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(editButton)(ctx); err != nil {
			if err := ui.Exists(notificationTray)(ctx); err == nil {
				if err := ui.LeftClick(notificationTray)(ctx); err != nil {
					return errors.Wrap(err, "failed to click notification center tray")
				}
			}
			if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(editButton)(ctx); err != nil {
				if err := ui.Exists(expandButton)(ctx); err == nil {
					_ = ui.LeftClick(expandButton)(ctx)
				}
			}
		}
		return ui.WithTimeout(5 * time.Second).LeftClick(editButton)(ctx)
	}

	if err := clickEdit(ctx); err != nil {
		return errors.Wrap(err, "failed to click edit button")
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, 15*time.Second); err != nil {
		testing.ContextLog(ctx, "Gallery app not open after 15s; retrying Edit button from Notification Center: ", err)
		_ = clickEdit(ctx)
		if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, 45*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for Gallery app to open")
		}
	}
	return nil
}
