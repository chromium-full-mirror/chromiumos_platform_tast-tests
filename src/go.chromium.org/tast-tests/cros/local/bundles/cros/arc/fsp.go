// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/storage"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
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

// arc.Fsp / arc.Fsp.vm tast tests make use of an unarchiver to mount a test zip file
// on a pseudo file system, accessible by FSP. An Android app is then used to read a
// file on this file system via FSP.

const (
	// fspZipFile is the name of the test zip file.
	fspZipFile = "arc_fsp_storage.zip"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Fsp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Android app can read files on pseudo file systems using File System Provider (FSP) via FilesApp",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Data:         []string{fspZipFile},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
	})
}

// Fsp implements the test scenario of arc.Fsp.
func Fsp(ctx context.Context, s *testing.State) {
	const (
		// These need to match with the one contained in |fspZipFile|.
		filename    = "storage.txt"
		fileContent = "this is a test"

		unarchiverName = "Wicked Good Unarchiver"
		unarchiverURL  = "https://chrome.google.com/webstore/detail/wicked-good-unarchiver/mljpablpddhocfbnokacjggdbmafjnon?hl=en"
	)

	// GAIA login is required to use Chrome Web Store.
	cr, err := chrome.New(
		ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Ensure the existence of Text app. This is because TestFilesAppIntegration expects that
	// there is at least one app (other than ArcFileEditorTest, the Android app installed in
	// TestFilesAppIntegration) that can open a text file.
	if err := installTextAppIfNotInstalled(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to ensure the existence of Text app: ", err)
	}

	// Install the unarchiver Chrome app, that supports FSP.
	app := cws.App{Name: unarchiverName, URL: unarchiverURL}
	if err := cws.InstallApp(ctx, cr.Browser(), tconn, app); err != nil {
		s.Fatal("Chrome app installation failed: ", err)
	}

	userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatalf("Failed to get the cryptohome user path for %s: %v", cr.NormalizedUser(), err)
	}

	destPath := filepath.Join(userPath, "MyFiles", fspZipFile)
	if err := fsutil.CopyFile(s.DataPath(fspZipFile), destPath); err != nil {
		s.Fatalf("Failed to copy %s to %s: %v", fspZipFile, destPath, err)
	}

	// By unzipping, it will create a pseudo file system accessible by FSP.
	if err := unzipFile(ctx, cr, tconn, fspZipFile, filesapp.MyFiles, unarchiverName, s.OutDir()); err != nil {
		s.Fatal("Unzip test zip file failed: ", err)
	}

	config := storage.TestConfig{
		DirName:     fspZipFile,
		FileName:    filename,
		FileContent: fileContent,
		OutDir:      s.OutDir(),
		ReadOnly:    true,
	}
	if err := storage.TestFilesAppIntegration(ctx, a, cr, d, config); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}

func installTextAppIfNotInstalled(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	const (
		textAppName = "Text"
		textAppURL  = "https://chrome.google.com/webstore/detail/text/mmfbcljfglbokpmkimbfghdkjmjhdgbg"
		textAppID   = "mmfbcljfglbokpmkimbfghdkjmjhdgbg"
	)

	installed, err := ash.ChromeAppInstalled(ctx, tconn, textAppID)
	if err != nil {
		return errors.Wrap(err, "failed to check the existence of Text app")
	}
	if installed {
		return nil
	}
	testing.ContextLog(ctx, "Installing the missing Text app")
	textApp := cws.App{Name: textAppName, URL: textAppURL}
	return cws.InstallApp(ctx, cr.Browser(), tconn, textApp)
}

// unzipFile unzips the specified "zipFile" located at "folder" using the "unarchiver".
func unzipFile(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, zipFile, folder, unarchiver, outDir string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	msg := "Opening the test zip file with " + unarchiver
	testing.ContextLog(ctx, msg)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "launching the Files App failed")
	}
	// Close the Files App window to avoid having two windows in TestOpenWithAndroidApp.
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, func() bool { return retErr != nil }, cr, "unzip_failure")

	return uiauto.Combine(msg,
		files.OpenPath(filesapp.FilesTitlePrefix+folder, folder),
		files.SelectFile(zipFile),
		files.LeftClick(nodewith.Name("Open").Role(role.Button)),
		files.LeftClick(nodewith.Name(unarchiver).Role(role.StaticText)),
		// Wait until the unzipped file appears in the navigation tree.
		files.WithTimeout(time.Minute).WaitUntilExists(nodewith.Name(zipFile).Role(role.TreeItem)),
	)(ctx)
}
