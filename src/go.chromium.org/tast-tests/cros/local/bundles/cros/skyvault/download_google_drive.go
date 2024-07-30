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

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DownloadGoogleDrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		LacrosStatus:   testing.LacrosVariantUnneeded,
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
	})
}

// DownloadGoogleDrive tests that downloads are saved to GoogleDrive when forced by policy.
func DownloadGoogleDrive(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	username, password, err := dma.UserPassFromPool(policy.ManagedUserAccountPoolVarName)
	if err != nil {
		s.Fatal("Failed to get username and password: ", err)
	}

	policies := []policy.Policy{
		&policy.DownloadDirectory{Val: "${google_drive}"},
		&policy.DriveDisabled{Val: false},
	}
	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), username, policies)
	if err != nil {
		s.Fatal("Could not set set up fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	chromeOptions := []chrome.Option{
		chrome.EnableFeatures("SkyVault"),
		chrome.DMSPolicy(fdms.URL),
		chrome.GAIALogin(chrome.Creds{
			User: username,
			Pass: password,
		}),
	}

	cr, err := chrome.New(ctx, chromeOptions...)
	if err != nil {
		s.Fatal("Connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "download_google_drive")
	s.AttachErrorHandlers(handler, handler)

	dfs, err := drivefs.NewDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to wait for DriveFS to mount: ", err)
	}
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	if err := files.OpenDrive()(ctx); err != nil {
		s.Fatal("Failed to open Drive: ", err)
	}
	if err := files.FileExists("data.txt"); err == nil {
		if err := files.DeleteFileOrFolder(kb, "data.txt")(ctx); err != nil {
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
	if err := conn.Eval(ctx, `document.getElementById('data.txt').click()`, nil); err != nil {
		s.Fatal("Failed to execute JS expression: ", err)
	}

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
	filename, err := files.WaitForFileByPattern(ctx, regexp.MustCompile("^data.txt$"))
	if err != nil {
		s.Fatal("Downloaded file not found on Drive: ", err)
	}

	if err := files.DeleteFileOrFolder(kb, filename)(ctx); err != nil {
		s.Fatal("Failed to delete the file: ", err)
	}
}
