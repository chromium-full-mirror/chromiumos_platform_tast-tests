// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/files"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesCrostini,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file Crostini restriction",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > DLP (Data Loss Prevention)
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome", "vm_host", "dlc"},
		HardwareDeps: crostini.CrostiniStable,
		Attr:         []string{"group:mainline", "informational"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
		},
		Fixture: "crostiniBullseyePolicy",
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesCrostini(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(crostini.FixtureData).Chrome
	fakeDMS := s.FixtValue().(crostini.FixtureData).FakeDMS
	keyboard := s.FixtValue().(crostini.FixtureData).KB
	tconn := s.FixtValue().(crostini.FixtureData).Tconn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	// Update the policy.
	filesCrostiniWarnPolicy := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before transferring a confidential file to Linux files",
				Description: "User should be warned before transferring a confidential file to Linux files",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"CROSTINI",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "WARN",
					},
				},
			},
		},
	},
	}

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, filesCrostiniWarnPolicy); err != nil {
		s.Fatal("Failed to serve and verify policy: ", err)
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Fatal("Failed to clear Downloads directory: ", err)
	}

	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// Download the file.
	if err := files.DownloadFile(ctx, tconn, cr.Browser(), s.DataFileSystem()); err != nil {
		s.Fatal("Failed to download file: ", err)
	}

	// Open the Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	if err := filesApp.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads: ", err)
	}

	// Start interacting with the UI.
	ui := uiauto.New(tconn)

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, files.DlFileName, true); err != nil {
		s.Fatal("File isn't managed when it should be: ", err)
	}

	// Copy the file to Linux then cancel the warning.
	if err := copyToLinuxFiles(ctx, ui, filesApp, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to copy to Linux files: ", err)
	}

	if err := files.CancelWarningAndVerify(ctx, ui, tconn, files.DlFileName); err != nil {
		s.Fatal("Failed to cancel the warning: ", err)
	}

	// Copy the file to Linux then proceed the warning.
	if err := copyToLinuxFiles(ctx, ui, filesApp, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to copy to Linux files: ", err)
	}

	if err := files.AcceptWarningAndVerify(ctx, ui, tconn, files.DlFileName); err != nil {
		s.Fatal("Failed to proceed the warning: ", err)
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, files.DlFileName, false); err != nil {
		s.Fatal("File is managed when it shouldn't be: ", err)
	}

	// Delete the copied file before we try to copy again.
	if err := filesApp.DeleteFileOrFolder(keyboard, files.DlFileName)(ctx); err != nil {
		s.Error("Failed to delete file: ", err)
	}

	// Copy again. Warning should be silently bypassed and the file should appear.
	if err := copyToLinuxFiles(ctx, ui, filesApp, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to copy to Linux files: ", err)
	}
	if err := filesApp.WithTimeout(10 * time.Second).WaitForFile(files.DlFileName)(ctx); err != nil {
		s.Error("Failed to wait for file: ", err)
	}
}

// copyToLinuxFiles tries to copy a file to Linux files.
func copyToLinuxFiles(ctx context.Context, ui *uiauto.Context, f *filesapp.FilesApp, kb *input.KeyboardEventWriter, filename string) error {
	if err := uiauto.Combine("copy the file to Linux files",
		f.OpenDownloads(),
		f.CopyFileToClipboard(filename),
		f.OpenLinuxFiles(),
		f.PasteFileFromClipboard(kb),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy the file to Linux files")
	}

	return nil
}
