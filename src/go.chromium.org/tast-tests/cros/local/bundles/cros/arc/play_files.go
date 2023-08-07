// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/storage"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayFiles,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks whether the Play files directory is properly shared from ARC to ChromeOS",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"capybara.jpg"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Val:               "/run/arc/sdcard/write/emulated/0",
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val:               "/media/fuse/android_files",
		}},
		Timeout: 6 * time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

// PlayFiles implements the test scenario of arc.PlayFiles.
func PlayFiles(ctx context.Context, s *testing.State) {
	crosPlayfilesPath := s.Param().(string)

	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	args := arc.DisableSyncFlags()
	cr, err := chrome.New(
		ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.ExtraArgs(args...),
	)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Optin is needed to enable the Play files feature.
	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := arc.WaitForARCSDCardVolumeMount(ctx, a); err != nil {
		s.Fatal("Failed to wait for the sdcard volume to be mounted in ARC: ", err)
	}

	testing.ContextLog(ctx, "Testing storage integration with apps")
	if err := testStorageIntegrationForPlayfilesWithApps(ctx, cr, tconn, a, s.OutDir()); err != nil {
		s.Fatal("Storage integration test with apps failed: ", err)
	}

	testing.ContextLog(ctx, "Testing Android -> CrOS")
	if err := testAndroidToCros(ctx, a, s.DataPath("capybara.jpg"), crosPlayfilesPath); err != nil {
		s.Fatal("Android -> CrOS failed: ", err)
	}
}

// testStorageIntegrationForPlayfilesWithApps tests the following scenario:
//  1. Create a file in Play files with Files app and read it from Android side.
//  2. Open the file with a test Android app via Files app's "Open with..." menu,
//     edit the file with the app, and verify the modification with Files app.
//  3. Open the file with a test Android app via SAF.
//  4. Delete the file with Files app and verify the deletion from Android side.
func testStorageIntegrationForPlayfilesWithApps(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, outDir string) error {
	const (
		filename    = "storage.txt"
		fileContent = "this is a test"
	)

	if err := testCopyToPlayfiles(ctx, cr, tconn, a, filename, fileContent, outDir); err != nil {
		return errors.Wrapf(err, "failed to copy %s to Play files", filename)
	}

	if err := testFilesAppIntegrationForPlayfiles(ctx, cr, a, filename, fileContent, outDir); err != nil {
		return errors.Wrapf(err, "failed to test Files app integration for %s in Play files", filename)
	}

	if err := testDeleteFromPlayfiles(ctx, tconn, a, filename, outDir); err != nil {
		return errors.Wrapf(err, "failed to delete %s from Play files", filename)
	}

	return nil
}

// testCopyToPlayfiles writes a file to the ChromeOS Downloads directory and
// copies it to Play files through the Files app. It also checks that the copied
// file appears on the Android side.
func testCopyToPlayfiles(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, filename, fileContent, outDir string) error {
	expected := []byte(fileContent)
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	crosPath := filepath.Join(downloadsPath, filename)
	if err := ioutil.WriteFile(crosPath, expected, 0644); err != nil {
		return errors.Wrapf(err, "failed to write to %s in ChromeOS", crosPath)
	}
	defer os.Remove(crosPath)

	if err := copyFileInDownloadsToPlayfiles(ctx, tconn, filename, outDir); err != nil {
		return errors.Wrapf(err, "failed to copy %s through the Files app", filename)
	}

	// Check that the file appears on the Android side.
	androidPath := filepath.Join("/storage/emulated/0/Pictures", filename)
	return testing.Poll(ctx, func(ctx context.Context) error {
		actual, err := a.ReadFile(ctx, androidPath)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s in Android", androidPath)
		}
		if !bytes.Equal(actual, expected) {
			return errors.Errorf("content mismatch between %s in Android and %s in ChromeOS", androidPath, crosPath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// copyFileInDownloadsToPlayfiles copies a file in Downloads to Play files.
func copyFileInDownloadsToPlayfiles(ctx context.Context, tconn *chrome.TestConn, filename, outDir string) (retErr error) {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch the Files app")
	}
	defer filesApp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, tconn, "copy_to_playfiles")

	steps := []uiauto.Action{
		// Open "Downloads".
		filesApp.OpenDownloads(),

		// Wait for file to be displayed.
		filesApp.WaitForFile(filename),

		// Copy file.
		filesApp.ClickContextMenuItem(filename, filesapp.Copy),

		// Open "Play files".
		filesApp.OpenPlayfiles(),

		// Paste to "Pictures" directory.
		filesApp.ClickContextMenuItem("Pictures", "Paste into folder"),

		// Open "Pictures".
		filesApp.OpenFile("Pictures"),

		// Wait until file exists.
		filesApp.WaitForFile(filename)}

	return uiauto.Combine("copy file from Downloads to Play files", steps...)(ctx)
}

// testFilesAppIntegrationForPlayfiles opens a file in Play files with an Android app and edits it.
func testFilesAppIntegrationForPlayfiles(ctx context.Context, cr *chrome.Chrome, a *arc.ARC, filename, fileContent, outDir string) error {
	d, err := a.NewUIDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize UI Automator")
	}

	config := storage.TestConfig{
		DirName:        filesapp.Playfiles,
		SubDirectories: []string{"Pictures"},
		FileName:       filename,
		FileContent:    fileContent,
		OutDir:         outDir,
		ReadOnly:       false,
	}
	return storage.TestFilesAppIntegration(ctx, a, cr, d, config)
}

// testDeleteFromPlayfiles deletes a file in Play files through the Files app.
// It also checks that the file is properly deleted on the Android side.
func testDeleteFromPlayfiles(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, filename, outDir string) (retErr error) {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to open Files app")
	}
	defer filesApp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, tconn, "delete_from_playfiles")

	steps := []uiauto.Action{
		// Open "Play files".
		filesApp.OpenPlayfiles(),

		// Open "Pictures".
		filesApp.OpenFile("Pictures"),

		// Wait for file to appear.
		filesApp.WaitForFile(filename),

		// Delete the file. This also waits for the file to be gone.
		filesApp.DeleteFileOrFolder(kb, filename)}

	if err := uiauto.Combine("delete file from Play files", steps...)(ctx); err != nil {
		return err
	}

	// Check that the file is deleted on the Android side.
	androidPath := filepath.Join("/storage/emulated/0/Pictures", filename)
	return testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := a.ReadFile(ctx, androidPath); err == nil {
			return errors.Errorf("%s still exists in Android", androidPath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// testAndroidToCros creates a file in Android's sdcard volume, and checks that
// the file appears in the corresponding ChromeOS side path.
func testAndroidToCros(ctx context.Context, a *arc.ARC, dataPath, crosPlayfilesPath string) error {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const androidPath = "/storage/emulated/0/Pictures/capybara.jpg"
	crosPath := filepath.Join(crosPlayfilesPath, "Pictures", "capybara.jpg")

	expected, err := ioutil.ReadFile(dataPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", dataPath)
	}

	if err := a.WriteFile(ctx, androidPath, expected); err != nil {
		return errors.Wrapf(err, "failed to write to %s in Android", androidPath)
	}
	defer a.RemoveAll(cleanupCtx, androidPath)

	actual, err := ioutil.ReadFile(crosPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s in ChromeOS", crosPath)
	}

	if !bytes.Equal(actual, expected) {
		return errors.Wrapf(err, "content mismatch between %s in ChromeOS and %s in Android", crosPath, androidPath)
	}

	return nil
}
