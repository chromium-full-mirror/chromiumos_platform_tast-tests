// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ScreenshotOnedrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving screenshots to OneDrive when ScreenCaptureLocation policy is set",
		BugComponent:   "b:1533988",
		Contacts: []string{
			"poromov@chromium.org",
			"aidazolic@chromium.org",
			"cros-commercial-clippy-eng@google.com",
		},
		SoftwareDeps: []string{
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
		Fixture: "onedriveManagedWithSkyVault",
		Timeout: 4 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
			pci.SearchFlag(&policy.ScreenCaptureLocation{}, pci.Served),
		},
	})
}

// ScreenshotOnedrive tests that screenshots are saved to OneDrive when forced by policy.
func ScreenshotOnedrive(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "screenshot_onedrive")
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

	if err := onedrive.DeleteFilesInRootByPrefix(ctx, "Screenshot"); err != nil {
		s.Fatal("Failed to remove old screenshots in OneDrive: ", err)
	}
	onedrive.DeleteDirsInRootByPrefixes(ctx, "Camera_", "ChromeOS device")

	// Set OneDrive and SkyVault policies.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.ScreenCaptureLocation{Val: "${microsoft_onedrive}"},
		&policy.MicrosoftOneDriveMount{Val: "allowed"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	layout, err := input.KeyboardTopRowLayout(ctx, keyboard)
	if err != nil {
		s.Fatal("Failed to retrieve keyboard top row layout: ", err)
	}

	// Press Ctrl+F5 to take the screenshot.
	if err := keyboard.Accel(ctx, "Ctrl+"+layout.SelectTask); err != nil {
		s.Fatal("Failed to type screenshot hotkey: ", err)
	}
	defer func(ctx context.Context) {
		if err := onedrive.DeleteFilesInRootByPrefix(ctx, "Screenshot"); err != nil {
			testing.ContextLog(ctx, "Failed to clean up screenshots in OneDrive: ", err)
		}
	}(cleanupCtx)

	// Verify that screenshot is successfully taken.
	if _, err := ash.WaitForNotification(ctx, tconn, time.Minute, ash.WaitIDContains("capture_mode_notification"), ash.WaitTitle("Screenshot taken")); err != nil {
		s.Errorf("Failed to wait for notification with title \"%q\": %v", "capture_mode_notification", err)
	}

	// Close all notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	if err := onedrive.OpenOneDrive(files)(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Verify that the screenshot is saved to OneDrive.
	if err := onedrive.WaitForFileByPattern(ctx, files, regexp.MustCompile(`^Screenshot.*\.png$`)); err != nil {
		s.Fatal("Screenshot not found on OneDrive: ", err)
	}
}
