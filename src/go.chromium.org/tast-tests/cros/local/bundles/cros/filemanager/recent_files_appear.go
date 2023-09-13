// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const testImage = "files_app_test.png"

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecentFilesAppear,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the edited files are shown in Recent tab",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"jinrongwu@google.com",
		},
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{testImage},
		Fixture:      "chromeLoggedIn",
	})
}

// RecentFilesAppear checks the edited files are shown in Recent tab.
func RecentFilesAppear(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Get Test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files App: ", err)
	}
	defer files.Close(cleanupCtx)

	testPath := s.DataPath(testImage)

	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve users MyFiles path: ", err)
	}
	downloadsPath := filepath.Join(myFilesPath, "Downloads")

	for _, subtest := range []struct {
		dirName string
		dirPath string
	}{
		{filesapp.Downloads, downloadsPath},
		{filesapp.MyFiles, myFilesPath},
	} {
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			imageFileLocation := filepath.Join(subtest.dirPath, testImage)
			if err := fsutil.CopyFile(testPath, imageFileLocation); err != nil {
				s.Fatalf("Failed to copy the test image to %s: %v", imageFileLocation, err)
			}
			defer os.Remove(imageFileLocation)

			dumpName := "ui_" + strings.ReplaceAll(subtest.dirName, " ", "")
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, dumpName)

			ui := uiauto.New(tconn)
			if err := prepareFile(ctx, ui, files, testImage, subtest.dirPath); err != nil {
				s.Fatal("Failed to prepare test file: ", err)
			}

			if err := updateModificationTimeToNow(ctx, tconn, subtest.dirPath, testImage); err != nil {
				s.Fatal("Failed to change the file modification date: ", err)
			}

			testing.ContextLog(ctx, "Refresh until file exists in Recent")
			if err := ui.WithTimeout(30*time.Second).RetryUntil(
				refreshRecent(files),
				files.WithTimeout(5*time.Second).WaitForFile(testImage),
			)(ctx); err != nil {
				s.Fatal("Failed to find file in recent: ", err)
			}

			testing.ContextLog(ctx, "Refresh until file exists in Recent Images")
			if err := ui.WithTimeout(30*time.Second).RetryUntil(
				goToRecentImages(files),
				files.WithTimeout(5*time.Second).WaitForFile(testImage),
			)(ctx); err != nil {
				s.Fatal("Failed to find file in recent images: ", err)
			}
		}

		if !s.Run(ctx, subtest.dirName, f) {
			s.Errorf("Failed to run test in %q", subtest.dirName)
		}
	}
}

// prepareFile creates a new file and change its modification date to 1 year
// ago, so it won't be appeared in the Recent view.
func prepareFile(ctx context.Context, ui *uiauto.Context, files *filesapp.FilesApp, filename, folderPath string) error {
	aYearBefore := time.Now().Local().AddDate(-1, 0, 0)
	formattedTime := aYearBefore.Format("200601021504")

	// Change the modified date to ensure the file is not shown in Recent tab.
	filePath := filepath.Join(folderPath, filename)
	if _, err := testexec.CommandContext(ctx, "touch", "-t", formattedTime, filePath).Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run command to change edit date")
	}

	testing.ContextLog(ctx, "Refresh until file gone in Recent")
	return ui.RetryUntil(
		refreshRecent(files),
		files.WithTimeout(2*time.Second).WaitUntilFileGone(filename),
	)(ctx)
}

// updateModificationTimeToNow updated the file modification date to the now
// time so it can be appeared in the Recent view.
func updateModificationTimeToNow(ctx context.Context, tconn *chrome.TestConn, folderPath, filename string) error {
	filePath := filepath.Join(folderPath, filename)
	nowTime := time.Now().Local()
	return os.Chtimes(filePath, nowTime, nowTime)
}

// refreshRecent refresh the recent page by switching between directories.
func refreshRecent(files *filesapp.FilesApp) uiauto.Action {
	return uiauto.Combine("refresh recent tab",
		files.OpenDownloads(),
		files.OpenDir(filesapp.Recent, filesapp.FilesTitlePrefix+filesapp.Recent),
	)
}

// goToRecentImages navigate to the recent image view by opening the Recent
// menu and clicking the Images filter button.
func goToRecentImages(files *filesapp.FilesApp) uiauto.Action {
	return uiauto.Combine("go to recent images by filter button",
		files.LeftClick(nodewith.Name("All").Role(role.ToggleButton)),
		files.LeftClick(nodewith.Name("Images").Role(role.ToggleButton)),
	)
}
