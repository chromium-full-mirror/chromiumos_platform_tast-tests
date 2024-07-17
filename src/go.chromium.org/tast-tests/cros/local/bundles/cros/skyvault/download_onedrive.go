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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DownloadOnedrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		LacrosStatus:   testing.LacrosVariantUnneeded,
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
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
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

	// Verify that file is successfully downloaded.
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitTitle("Download complete")); err != nil {
		s.Errorf("Failed to wait for notification with title \"%q\": %v", "Download complete", err)
	}

	files, err = filesapp.Relaunch(ctx, tconn, files)
	if err != nil {
		s.Fatal("Failed to relaunch Files app: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Verify that the downloaded file is saved to OneDrive.
	filename, err := files.FindFileByPattern(ctx, regexp.MustCompile("^data.txt$"))
	if err != nil {
		s.Fatal("Downloaded file not found on OneDrive: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}
	if err := files.DeleteFileOrFolder(kb, filename)(ctx); err != nil {
		s.Fatal("Failed to delete the file: ", err)
	}
}
