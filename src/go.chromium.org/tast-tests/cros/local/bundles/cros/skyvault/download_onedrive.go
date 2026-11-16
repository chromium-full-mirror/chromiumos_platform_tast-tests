// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DownloadOnedrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving downloads to OneDrive when DownloadDirectory policy is set",
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
			pci.SearchFlag(&policy.DownloadDirectory{}, pci.Served),
		},
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

// DownloadOnedrive tests that downloads are saved to OneDrive when forced by policy.
func DownloadOnedrive(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "download_onedrive")
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

	if err := onedrive.DeleteFilesInRootByPrefix(ctx, "data.txt"); err != nil {
		s.Fatal("Failed to remove old downloads in OneDrive: ", err)
	}
	onedrive.DeleteDirsInRootByPrefixes(ctx, "Camera_", "ChromeOS device")

	// Set OneDrive and SkyVault policies.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.DownloadDirectory{Val: "${microsoft_onedrive}"},
		&policy.MicrosoftOneDriveMount{Val: "allowed"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Open the local page with the file to download.
	conn, err := cr.NewConn(ctx, server.URL+"/download.html")
	if err != nil {
		s.Fatal("Failed to open browser: ", err)
	}
	defer conn.Close()

	testing.ContextLog(ctx, "Opened the browser")

	// The file name is also the ID of the link element, download it.
	if err := conn.Eval(ctx, `document.getElementById('data.txt').click()`, nil); err != nil {
		s.Fatal("Failed to execute JS expression: ", err)
	}
	defer func(ctx context.Context) {
		if err := onedrive.DeleteFilesInRootByPrefix(ctx, "data.txt"); err != nil {
			testing.ContextLog(ctx, "Failed to clean up downloaded file in OneDrive: ", err)
		}
	}(cleanupCtx)

	// Verify that file is successfully downloaded.
	if _, err := ash.WaitForNotification(ctx, tconn, time.Minute, ash.WaitTitle("Download complete")); err != nil {
		s.Errorf("Failed to wait for notification with title \"%q\": %v", "Download complete", err)
	}

	files, err = filesapp.Relaunch(ctx, tconn, files)
	if err != nil {
		s.Fatal("Failed to relaunch Files app: ", err)
	}

	if err := onedrive.OpenOneDrive(files)(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Verify that the downloaded file is saved to OneDrive.
	if err := onedrive.WaitForFileByPattern(ctx, files, regexp.MustCompile("^data.txt$")); err != nil {
		s.Fatal("Downloaded file not found on OneDrive: ", err)
	}

	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}
}
