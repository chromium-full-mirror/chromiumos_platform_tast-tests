// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ScreenshotGoogleDrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving screenshots to Google Drive when ScreenCaptureLocation policy is set",
		BugComponent:   "b:1533988",
		Contacts: []string{
			"poromov@chromium.org",
			"aidazolic@chromium.org",
			"cros-commercial-clippy-eng@google.com",
		},
		SoftwareDeps: []string{
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
			pci.SearchFlag(&policy.ScreenCaptureLocation{}, pci.Served),
		},
		Fixture: "driveFsManagedWithSkyVault",
	})
}

// ScreenshotGoogleDrive tests that screenshots are saved to Google Drive when forced by policy.
func ScreenshotGoogleDrive(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs
	driveAPIClient := fixt.APIClient

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "screenshot_google_drive")
	s.AttachErrorHandlers(handler, handler)

	recorder, err := uiauto.NewScreenRecorder(ctx, tconn, fixt.Chrome)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
	}
	if recorder != nil {
		if err := recorder.Start(ctx, tconn); err != nil {
			s.Log("Failed to start screen recorder: ", err)
		}
		defer recorder.StopAndSaveOnError(cleanupCtx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	// Using the virtual keyboard is required since different physical keyboards can require different key combinations to take a screenshot.
	vkb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}
	defer vkb.Close(cleanupCtx)

	layout, err := input.KeyboardTopRowLayout(ctx, vkb)
	if err != nil {
		s.Fatal("Failed to retrieve keyboard top row layout: ", err)
	}

	// Press Ctrl+F5 to take the screenshot.
	if err := vkb.Accel(ctx, "Ctrl+"+layout.SelectTask); err != nil {
		s.Fatal("Failed to type screenshot hotkey: ", err)
	}

	// Verify that screenshot is successfully taken.
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains("capture_mode_notification"), ash.WaitTitle("Screenshot taken")); err != nil {
		s.Errorf("Failed to wait for notification with title \"%q\": %v", "capture_mode_notification", err)
	}

	if err := files.OpenDrive()(ctx); err != nil {
		s.Fatal("Failed to open Drive: ", err)
	}

	// Verify that the screenshot is saved to Google Drive.
	filename, err := files.WaitForFileByPattern(ctx, regexp.MustCompile(`^Screenshot.*\.png$`))
	if err != nil {
		s.Fatal("Screenshot not found on Drive: ", err)
	}
	defer func(ctx context.Context) {
		driveFilePath := dfs.MyDrivePath(filename)
		if err := os.Remove(driveFilePath); err != nil {
			testing.ContextLogf(ctx, "Failed to remove %s: %v", filename, err)
		}
		if err := drivefs.RemoveDriveFsFileViaAPI(dfs, driveAPIClient, filename)(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to remove %s via Drive API: %v", filename, err)
		}
	}(cleanupCtx)

	// Click edit button, check if Gallery app opens up.
	editButton := nodewith.NameRegex(regexp.MustCompile("(?i)edit")).Role(role.Button)
	ui := uiauto.New(tconn)
	if err := ui.LeftClick(editButton)(ctx); err != nil {
		s.Fatal("Failed to click edit button: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		s.Error("Failed to wait for Gallery app to open: ", err)
	}
}
