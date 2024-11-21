// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DownloadGoogleDrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verifies saving downloads to Google Drive when DownloadDirectory policy is set",
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
			pci.SearchFlag(&policy.DownloadDirectory{}, pci.Served),
		},
		Data: []string{
			"download.html",
			"data.txt",
		},
		Fixture: "driveFsManagedWithSkyVault",
	})
}

// DownloadGoogleDrive tests that downloads are saved to GoogleDrive when forced by policy.
func DownloadGoogleDrive(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	cr := fixt.Chrome
	dfs := fixt.DriveFs
	driveAPIClient := fixt.APIClient
	filename := "data.txt"

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "download_google_drive")
	s.AttachErrorHandlers(handler, handler)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	if err := files.OpenDrive()(ctx); err != nil {
		s.Fatal("Failed to open Drive: ", err)
	}
	if err := files.FileExists(filename); err == nil {
		if err := files.DeleteFileOrFolder(kb, filename)(ctx); err != nil {
			s.Fatal("Failed to remove old downloads in Drive: ", err)
		}
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
	if err := conn.Eval(ctx, `document.getElementById('`+filename+`').click()`, nil); err != nil {
		s.Fatal("Failed to execute JS expression: ", err)
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

	// Verify that file is successfully downloaded.
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitTitle("Download complete")); err != nil {
		s.Errorf("Failed to wait for notification with title \"%q\": %v", "Download complete", err)
	}

	files, err = filesapp.Relaunch(ctx, tconn, files)
	if err != nil {
		s.Fatal("Failed to relaunch Files app: ", err)
	}

	if err := files.OpenDrive()(ctx); err != nil {
		s.Fatal("Failed to open Drive: ", err)
	}

	// Verify that the downloaded file is saved to Drive.
	if _, err := files.WaitForFileByPattern(ctx, regexp.MustCompile(filename)); err != nil {
		s.Fatal("Downloaded file not found on Drive: ", err)
	}
}
