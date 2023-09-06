// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsCopyMove,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies file/folder copy/move operations work in ODFS",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"lucmult@chromium.org",
			"wenbojie@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		VarDeps: []string{
			"onedrive.accountPool",
		},
		Data: []string{
			"test_1KB.txt",
		},
		Params: []testing.Param{{
			Fixture: "onedrive",
		}, {
			Name:              "lacros",
			Fixture:           "onedriveLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// locationType is the type of the copy/move location.
type locationType string

const (
	local    locationType = "Local"
	oneDrive locationType = "OneDrive"
)

// copyMoveTestOptions contains the options for defining sub test of copy/move.
type copyMoveTestOptions struct {
	name     string
	source   locationType
	target   locationType
	isFolder bool
	isMove   bool
}

// OdfsCopyMove tests the file/folder copy/move operations work in ODFS.
func OdfsCopyMove(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_copy_move")

	// Connect to OneDrive.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := files.ConnectToOneDrive(ctx, ms365App); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard: ", err)
	}

	// Base path for local files.
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get users MyFiles path: ", err)
	}
	downloadsPath := filepath.Join(myFilesPath, filesapp.Downloads)

	// Base path for OneDrive files.
	odfsToken, err := files.GetOdfsFuseboxToken(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get ODFS key: ", err)
	}
	odfsFuseboxPath := filepath.Join(filemanager.FuseboxDirPath, odfsToken)

	subTests := []copyMoveTestOptions{
		{
			name:     "copyFileFromDownloadsToOneDrive",
			source:   local,
			target:   oneDrive,
			isFolder: false,
			isMove:   false,
		},
		{
			name:     "moveFileFromDownloadsToOneDrive",
			source:   local,
			target:   oneDrive,
			isFolder: false,
			isMove:   true,
		},
		{
			name:     "copyFolderFromDownloadsToOneDrive",
			source:   local,
			target:   oneDrive,
			isFolder: true,
			isMove:   false,
		},
		{
			name:     "moveFolderFromDownloadsToOneDrive",
			source:   local,
			target:   oneDrive,
			isFolder: true,
			isMove:   true,
		},
		{
			name:     "copyFileWithinOneDrive",
			source:   oneDrive,
			target:   oneDrive,
			isFolder: false,
			isMove:   false,
		},
		{
			name:     "moveFileWithinOneDrive",
			source:   oneDrive,
			target:   oneDrive,
			isFolder: false,
			isMove:   true,
		},
		{
			name:     "copyFolderWithinOneDrive",
			source:   oneDrive,
			target:   oneDrive,
			isFolder: true,
			isMove:   false,
		},
		{
			name:     "moveFolderWithinOneDrive",
			source:   oneDrive,
			target:   oneDrive,
			isFolder: true,
			isMove:   true,
		},
	}

	for _, test := range subTests {
		subTest := func(ctx context.Context, s *testing.State) {
			sourceName, sourceDirPath, cleanup, err := prepareSource(test, s.DataPath("test_1KB.txt"), downloadsPath, odfsFuseboxPath, odfsToken)
			if err != nil {
				s.Fatal("Failed to prepare source: ", err)
			}
			defer cleanup()
			targetDirPath, cleanup, err := prepareTarget(test, odfsFuseboxPath)
			if err != nil {
				s.Fatal("Failed to prepare target: ", err)
			}
			defer cleanup()
			isSameDir := !test.isMove && test.source == test.target
			if test.isMove {
				if err := moveFileOrFolder(files, kb, sourceName, sourceDirPath, targetDirPath)(ctx); err != nil {
					s.Fatal("Failed to move file or folder: ", err)
				}
			} else {
				if err := copyFileOrFolder(files, kb, sourceName, sourceDirPath, targetDirPath, isSameDir)(ctx); err != nil {
					s.Fatal("Failed to copy file or folder: ", err)
				}
			}
			// Source file cleanup is already handled by the above 2 cleanup calls.
			defer cleanupTargetAfterCopyOrMove(sourceName, test.target, isSameDir, downloadsPath, odfsFuseboxPath)
		}
		if !s.Run(ctx, test.name, subTest) {
			s.Errorf("Failed to run subtest %s", test.name)
		}
	}
}

// prepareSource creates the source file/folder based on the test options.
func prepareSource(options copyMoveTestOptions, srcFile, downloadsPath, odfsFuseboxPath, odfsToken string) (string, []string, func(), error) {
	var sourceDirPath []string
	var sourceDirToCreateFileOrFolder string
	if options.source == local {
		sourceDirPath = []string{filesapp.Downloads}
		sourceDirToCreateFileOrFolder = downloadsPath
	} else if options.source == oneDrive {
		sourceDirPath = []string{filesapp.OneDrive}
		if options.isFolder {
			sourceDirToCreateFileOrFolder = odfsFuseboxPath
		} else {
			// For creating file in OneDrive, we just need to pass odfsToken.
			sourceDirToCreateFileOrFolder = odfsToken
		}
	}

	if options.isFolder {
		sourceName, cleanup, err := createFolder(sourceDirToCreateFileOrFolder, options.source)
		if err != nil {
			return "", []string{}, func() {}, errors.Wrap(err, "failed to create a folder")
		}
		return sourceName, sourceDirPath, cleanup, nil
	}

	sourceName, cleanup, err := createFile(sourceDirToCreateFileOrFolder, srcFile, options.source)
	if err != nil {
		return "", []string{}, func() {}, errors.Wrap(err, "failed to create a file")
	}
	return sourceName, sourceDirPath, cleanup, nil
}

// prepareTarget returns the target folder based on the test options, it also creates
// additional folder if needed.
func prepareTarget(options copyMoveTestOptions, odfsFuseboxPath string) ([]string, func(), error) {
	var targetDirPath []string
	if options.target == local {
		targetDirPath = []string{filesapp.Downloads}
	} else if options.target == oneDrive {
		targetDirPath = []string{filesapp.OneDrive}
	}
	// Move within OneDrive requires creating a additional folder as the target folder.
	needAdditionalFolder := options.isMove && options.source == oneDrive && options.source == options.target
	if needAdditionalFolder {
		additionalDirName, cleanup, err := createFolder(odfsFuseboxPath, options.target)
		if err != nil {
			return []string{}, func() {}, errors.Wrap(err, "failed to create additional folder for moving")
		}
		targetDirPath = append(targetDirPath, additionalDirName)
		return targetDirPath, cleanup, nil
	}
	return targetDirPath, func() {}, nil
}

// createFile create a file and return the full path and the cleanup function.
func createFile(basePath, srcFile string, location locationType) (string, func(), error) {
	testFileName := filemanager.GenerateTestFileName("_odfs_text.txt")
	if location == local {
		testFilePath := filepath.Join(basePath, testFileName)
		if err := fsutil.CopyFile(srcFile, testFilePath); err != nil {
			return "", func() {}, errors.Wrap(err, "failed to copy test file")
		}
		cleanupFunc := func() { os.Remove(testFilePath) }
		return testFileName, cleanupFunc, nil
	} else if location == oneDrive {
		testFilePath, err := filemanager.CreateFileInFusebox(basePath, testFileName, "test")
		if err != nil {
			return "", func() {}, errors.Wrap(err, "failed to create test file in OneDrive")
		}
		cleanupFunc := func() { os.Remove(testFilePath) }
		return testFileName, cleanupFunc, nil
	}
	return "", func() {}, errors.Errorf("location type %q is not supported", location)
}

// createFolder create a folder and return the full path and the cleanup function.
func createFolder(basePath string, location locationType) (string, func(), error) {
	testDirName := filemanager.GenerateTestFileName("_odfs_folder")
	testDirPath := filepath.Join(basePath, testDirName)
	if err := os.MkdirAll(testDirPath, 0755); err != nil {
		return "", func() {}, errors.Wrap(err, "failed to create folder")
	}
	cleanupFunc := func() { os.RemoveAll(testDirPath) }
	if location == local {
		// The folders must be owned by `chronous` to ensure it can be deleted by
		// `filesapp` through UI control.
		if err := os.Chown(testDirPath, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
			return "", func() {}, errors.Wrapf(err, "failed to chown of folder: %q", testDirPath)
		}
		return testDirName, cleanupFunc, nil
	} else if location == oneDrive {
		// Fusebox file doesn't support Chown operation.
		return testDirName, cleanupFunc, nil
	}
	return "", func() {}, errors.Errorf("location type %q is not supported", location)
}

// copyFileOrFolder copies the file/folder from the source directory to the target directory.
func copyFileOrFolder(filesApp *filesapp.FilesApp, kb *input.KeyboardEventWriter, sourceName string, sourceDirPath, targetDirPath []string, isSameDir bool) uiauto.Action {
	copiedName := sourceName
	if isSameDir {
		copiedName = filemanager.GetCopiedName(sourceName, 1)
	}
	return uiauto.Combine("Copy file or folder",
		uiauto.Log(fmt.Sprintf("Copying %q from %q to %q", sourceName, sourceDirPath, targetDirPath)),
		filesApp.OpenPath(filesapp.FilesTitlePrefix+sourceDirPath[0], sourceDirPath[0], sourceDirPath[1:]...),
		filesApp.CopyFileToClipboard(sourceName),
		filesApp.OpenPath(filesapp.FilesTitlePrefix+targetDirPath[0], targetDirPath[0], targetDirPath[1:]...),
		filesApp.PasteFileFromClipboard(kb),
		// It takes time for the newly copied folder to appear in OneDrive.
		filesApp.WithTimeout(30*time.Second).WaitForFile(copiedName),
	)
}

// moveFileOrFolder moves the file/folder from the source directory to the target directory.
func moveFileOrFolder(filesApp *filesapp.FilesApp, kb *input.KeyboardEventWriter, sourceName string, sourceDirPath, targetDirPath []string) uiauto.Action {
	return uiauto.Combine("Move file or folder",
		uiauto.Log(fmt.Sprintf("Moving %q from %q to %q", sourceName, sourceDirPath, targetDirPath)),
		filesApp.OpenPath(filesapp.FilesTitlePrefix+sourceDirPath[0], sourceDirPath[0], sourceDirPath[1:]...),
		filesApp.CutFileToClipboard(sourceName),
		filesApp.OpenPath(filesapp.FilesTitlePrefix+targetDirPath[0], targetDirPath[0], targetDirPath[1:]...),
		filesApp.PasteFileFromClipboard(kb),
		// It takes time for the newly moved folder to appear in OneDrive.
		filesApp.WithTimeout(30*time.Second).WaitForFile(sourceName),
		// Check the source file/folder is disappeared.
		filesApp.OpenPath(filesapp.FilesTitlePrefix+sourceDirPath[0], sourceDirPath[0], sourceDirPath[1:]...),
		filesApp.WaitUntilFileGone(sourceName),
	)
}

// cleanupTargetAfterCopyOrMove deletes the copied/moved file/folder after copy/move.
func cleanupTargetAfterCopyOrMove(fileName string, targetLocation locationType, isSameDir bool, downloadsPath, odfsFuseboxPath string) {
	parentDirPath := downloadsPath
	if targetLocation == oneDrive {
		parentDirPath = odfsFuseboxPath
	}
	targetName := fileName
	if isSameDir {
		targetName = filemanager.GetCopiedName(fileName, 1)
	}
	os.RemoveAll(filepath.Join(parentDirPath, targetName))
}
