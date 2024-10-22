// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package skyvault

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
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
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MigrationOnedrive,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that local files are moved to OneDrive, when LocalUserFilesAllowed and LocalUserFilesMigrationDestination policies are set",
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
		Fixture: "onedriveManagedWithSkyVaultGA",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
			pci.SearchFlag(&policy.LocalUserFilesAllowed{}, pci.Served),
			pci.SearchFlag(&policy.LocalUserFilesMigrationDestination{}, pci.Served),
		},
	})
}

// MigrationOnedrive tests that local files are uploaded to OneDrive when forced by policy.
func MigrationOnedrive(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()
	targetBaseName := filepath.Base(data.TargetFolder)
	dataFiles := data.GeneratedFiles

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "migration_onedrive")
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

	testing.ContextLog(ctx, "Connected to OneDrive")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// TODO(crbug.com/372897049): Get the device ID to construct the full folder name,
	// so that different test runs don't interfere with each other.
	cloudFolderName := regexp.MustCompile("^ChromeOS device ")
	if err := deleteAllUploadFolders(ctx, files, kb, cloudFolderName); err != nil {
		s.Error("Failed to delete uploads folder: ", err)
	}

	// Set OneDrive and SkyVault policies.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.LocalUserFilesAllowed{Val: false},
		&policy.LocalUserFilesMigrationDestination{Val: "microsoft_onedrive"},
		&policy.MicrosoftOneDriveMount{Val: "allowed"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	ui := uiauto.New(tconn)
	dialog := nodewith.Role(role.Dialog).NameRegex(regexp.MustCompile("Your file upload to Microsoft OneDrive *"))
	uploadNowButton := nodewith.Ancestor(dialog).Role(role.Button).Name("Upload now")

	if err := uiauto.Combine("Upload now",
		uiauto.Log("Starting Upload now step"),
		ui.WaitUntilExists(dialog),
		ui.DoDefault(uploadNowButton),
		ui.WaitUntilGone(dialog),
	)(ctx); err != nil {
		s.Fatal("Failed to click Upload now: ", err)
	}

	// Verify that the migration completed.
	if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitTitleContains("Files were successfully uploaded to")); err != nil {
		s.Error("Failed to wait for migration completed notification: ", err)
	}

	if err := uiauto.Combine("Check that files are uploaded to OneDrive",
		files.OpenOneDrive(),
		files.OpenFile(cloudFolderName),
		files.OpenFile(targetBaseName),
		files.WithTimeout(10*time.Second).WaitForFile(dataFiles[0].FileName),
		files.WithTimeout(10*time.Second).WaitForFile(dataFiles[1].FileName),
		files.WithTimeout(10*time.Second).WaitForFile(dataFiles[2].FileName),
	)(ctx); err != nil {
		s.Error("Failed to find the uploaded files: ", err)
	}

	if err := deleteAllUploadFolders(ctx, files, kb, cloudFolderName); err != nil {
		s.Error("Failed to delete uploads folder: ", err)
	}
}

// deleteAllUploadFolders deletes all folders matching the given cloudFolderName pattern
// in the Files app.
func deleteAllUploadFolders(ctx context.Context, files *filesapp.FilesApp, kb *input.KeyboardEventWriter, cloudFolderName *regexp.Regexp) error {
	for {
		filename, err := files.WithTimeout(5*time.Second).WaitForFileByPattern(ctx, cloudFolderName)
		if err != nil {
			// No folders matching the pattern exist, exit the loop
			break
		}
		if err := uiauto.Combine("Delete upload folder",
			files.OpenOneDrive(),
			files.DeleteFileOrFolder(kb, filename),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to delete the uploads folder")
		}
	}
	return nil
}
