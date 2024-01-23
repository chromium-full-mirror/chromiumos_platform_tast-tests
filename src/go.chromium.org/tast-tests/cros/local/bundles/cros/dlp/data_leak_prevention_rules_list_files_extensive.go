// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/files"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	restrictedFile   = files.DlFileName
	unrestrictedFile = "Test file"
	folder           = "Test folder"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesExtensive,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with different file interactions",
		Timeout:      3 * time.Minute,
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcEnabled{}, pci.Served),
			pci.SearchFlag(&policy.PromptForDownloadLocation{}, pci.Served),
			pci.SearchFlag(&policy.TrashEnabled{}, pci.Served),
		},
		Params: []testing.Param{
			{
				Name:              "arc_container",
				ExtraAttr:         []string{"group:mainline", "informational"},
				Fixture:           fixture.ChromePolicyLoggedInARCFilesUXEnabled,
				ExtraSoftwareDeps: []string{"android_p"},
				Val:               "/run/arc/sdcard/write/emulated/0",
			}, {
				Name: "arc_vm",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				Fixture:           fixture.ChromePolicyLoggedInARCFilesUXEnabled,
				ExtraSoftwareDeps: []string{"android_vm"},
				Val:               "/media/fuse/android_files",
			},
		},
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesExtensive(ctx context.Context, s *testing.State) {
	// TODO(b/272255042): Test picker actions.
	// TODO(b/272256875): Test open&share actions.
	const (
		bootTimeout = 2 * time.Minute
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	playfilesPath := s.Param().(string)

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Update the policies.
	filesARCWarnPolicies := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Block before transferring a confidential file to Play files",
				Description: "Transferring a confidential file to Play files should be blocked",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"ARC",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "BLOCK",
					},
				},
			},
		},
	},
		&policy.ArcEnabled{Val: true, Stat: policy.StatusSet},
		&policy.PromptForDownloadLocation{Val: true},
		&policy.TrashEnabled{Val: true},
	}

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, filesARCWarnPolicies); err != nil {
		s.Fatal("Failed to serve and verify policies: ", err)
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Fatal("Failed to clear Downloads directory: ", err)
	}
	defer files.ClearDownloads(cleanupCtx, cr)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	// Setup ARC.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	// Create a Test folder in Downloads.
	folderPath := filepath.Join(downloadsPath, folder)
	if err := os.Mkdir(folderPath, 0777); err != nil {
		s.Error("Failed to create a test folder: ", err)
	}

	// Even though we pass 0777 to Mkdir, the created directory might not have these permissions, so we ensure that it does.
	if err := os.Chmod(folderPath, 0777); err != nil {
		s.Error("Failed to add write access to test folder: ", err)
	}

	// Create a local file in Downloads/Test folder.
	filePath := filepath.Join(folderPath, unrestrictedFile)
	if _, err := os.Create(filePath); err != nil {
		s.Error("Failed to create a test file: ", err)
	}

	// Open the Files app early so it is closed after the faillog screenshot.
	filesApp, err := files.LaunchFilesAppFullscreen(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	filesWin, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active Window for OS Settings: ", err)
	}

	// Write faillog before Files app and other windows get closed.
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	// Start interacting with the UI.
	ui := uiauto.New(tconn)

	if err := testDownload(ctx, ui, tconn, cr.Browser(), s.DataFileSystem()); err != nil {
		s.Fatal("Failed to testDownload: ", err)
	}

	if err := filesWin.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Cannot activate files window: ", err)
	}
	// Move the file to another local location and check that it's still managed,
	// and the created local file is not.
	if err := uiauto.Combine("move the file into the test folder",
		filesApp.OpenDownloads(),
		filesApp.SelectFile(restrictedFile),
		keyboard.AccelAction("Ctrl+X"),
		filesApp.ClickContextMenuItem(folder, "Paste into folder"),
		filesApp.OpenFile(folder),
		filesApp.WaitForFile(restrictedFile),
	)(ctx); err != nil {
		s.Fatal("Failed to move the file into test folder: ", err)
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, restrictedFile, true); err != nil {
		s.Fatal("Downloaded file isn't managed when it should be: ", err)
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, unrestrictedFile, false); err != nil {
		s.Fatal("Local file is managed when it shouldn't be: ", err)
	}

	if err := testCopyingToPlayfiles(ctx, ui, tconn, filesApp, playfilesPath); err != nil {
		s.Fatal("Failed to testCopyingToPlayfiles: ", err)
	}

	if err := testTrashingAndRestoring(ctx, ui, tconn, filesApp, keyboard); err != nil {
		s.Fatal("Failed to testTrashingAndRestoring: ", err)
	}
}

func testDownload(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, br *browser.Browser, dataFS http.FileSystem) error {
	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(dataFS))
	defer server.Close()

	if err := files.InitiateDownload(ctx, br, server); err != nil {
		return errors.Wrap(err, "failed to initiate download")
	}

	// Find the files app dialog.
	saver, err := filesapp.App(ctx, tconn, filesapp.FileSaverPseudoAppID)
	if err != nil {
		return errors.Wrap(err, "failed to locate the Files App")
	}

	// Saving to Play files/Pictures shouldn't be allowed.
	// This means that the "Open" button in the file saver is disabled.
	openButton := nodewith.Role(role.Button).Name("Open")

	if err := uiauto.Combine("select Play files",
		saver.OpenDir("My files", "My files"),
		saver.SelectFile("Play files"),
		saver.WaitUntilExists(openButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select Play files")
	}

	disabled, err := buttonDisabled(ctx, saver, openButton)
	if err != nil {
		return err
	}

	// It could happen that Play files aren't mounted yet, and won't be blocked by DLP yet.
	// Opening the Play files directory will mount it and all the subdirectories should be disabled.
	if !disabled {
		if err := uiauto.Combine("select Play files/Pictures",
			saver.OpenFile("Play files"),
			saver.SelectFile("Pictures"),
			saver.WaitUntilExists(openButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to open Play files")
		}

		disabled, err := buttonDisabled(ctx, saver, openButton)
		if err != nil {
			return err
		}

		if !disabled {
			return errors.New("open button is not disabled and it should be")
		}
	}

	saveButton := nodewith.Role(role.Button).Name("Save")
	// Save the file to Downloads.
	if err := uiauto.Combine("save the file",
		saver.OpenDir("Downloads", "Downloads"),
		saver.WaitUntilExists(saveButton),
		saver.LeftClick(saveButton), // Should be enabled, otherwise this will fail.
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to save the file")
	}

	return nil
}

func buttonDisabled(ctx context.Context, f *filesapp.FilesApp, button *nodewith.Finder) (bool, error) {
	info, err := f.Info(ctx, button)
	if err != nil {
		return false, errors.Wrap(err, "failed to find the button info")
	}

	return info.Restriction == restriction.Disabled, nil
}

func testCopyingToPlayfiles(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, f *filesapp.FilesApp, playFilesPath string) error {
	// Silently removing stale file in case it's present from other tests.
	os.Remove(playFilesPath + "/Pictures/" + restrictedFile)

	// The folder which contains a restricted file and a non-restricted file.
	if err := uiauto.Combine("copy the folder to Play files/Pictures",
		f.OpenDownloads(),
		f.CopyFileToClipboard(folder),
		f.OpenPlayfiles(),
		f.OpenFile("Pictures"),
		f.ClickDirectoryContextMenuItem("Pictures", "Paste into folder"),
		f.WithTimeout(10*time.Second).WaitForFile(folder))(ctx); err != nil {
		return errors.Wrap(err, "failed to copy the folder")
	}

	if err := uiauto.Combine("Verify the files in the copied folder",
		f.OpenFile(folder),
		f.WaitUntilFileGone(folder), // Ensure that navigated inside the folder.
		f.EnsureFileGone(restrictedFile, 10*time.Second),
		f.WaitForFile(unrestrictedFile))(ctx); err != nil {
		return errors.Wrap(err, "failed to verify the files")
	}

	return nil
}

func testTrashingAndRestoring(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, f *filesapp.FilesApp, kb *input.KeyboardEventWriter) error {
	// Trash the file and check it's still managed.
	if err := uiauto.Combine("trash the restricted file",
		f.OpenDownloads(),
		f.OpenFile(folder),
		f.TrashFileOrFolder(kb, restrictedFile),
		f.OpenTrash(),
		f.WaitForFile(restrictedFile),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to move file to trash")
	}

	if err := files.IsFileManaged(ctx, ui, tconn, kb, restrictedFile, true); err != nil {
		return errors.Wrap(err, "file isn't managed when it should be")
	}

	// Restore the file and check it's still managed.
	if err := uiauto.Combine("restore the downloaded file",
		kb.AccelAction("Search+Backspace"),
		f.OpenDownloads(),
		f.OpenFile(folder),
		f.WaitForFile(restrictedFile),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to restore the file")
	}

	if err := files.IsFileManaged(ctx, ui, tconn, kb, restrictedFile, true); err != nil {
		return errors.Wrap(err, "file isn't managed when it should be")
	}

	return nil
}
