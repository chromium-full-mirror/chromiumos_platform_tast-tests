// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/storage"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     MyFiles,
		Desc:     "Checks whether the MyFiles directory is properly shared from ChromeOS to ARC",
		Contacts: []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Data:         []string{"capybara.jpg"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational"},
			}},
		Timeout: 6 * time.Minute,
	})
}

// MyFiles implements the test scenario of arc.MyFiles.
func MyFiles(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	if err := arc.WaitForARCMyFilesVolumeMount(ctx, a); err != nil {
		s.Fatal("Failed to wait for MyFiles to be mounted in ARC: ", err)
	}

	cryptohomeUserPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatalf("Failed to get the cryptohome user path for %s: %v", cr.NormalizedUser(), err)
	}
	myFilesPath := cryptohomeUserPath + "/MyFiles"

	if err := testARCToCros(ctx, s, a, myFilesPath); err != nil {
		s.Fatal("Android -> CrOS failed: ", err)
	}

	if err := testFilesAppIntegrationForMyFiles(ctx, a, cr, d, myFilesPath, s.OutDir()); err != nil {
		s.Fatal("Files app integration test failed: ", err)
	}
}

// testARCToCros checks whether a file put in the Android MyFiles directory
// appears in the ChromeOS MyFiles directory.
func testARCToCros(ctx context.Context, s *testing.State, a *arc.ARC, myFilesPath string) error {
	const (
		filename    = "capybara.jpg"
		androidPath = "/storage/" + arc.MyFilesUUID + "/" + filename
	)
	crosPath := myFilesPath + "/" + filename

	testing.ContextLog(ctx, "Testing Android -> CrOS")

	return testPushToARCAndReadFromCros(ctx, a, s.DataPath(filename), androidPath, crosPath)
}

// testPushToARCAndReadFromCros pushes the content of sourcePath (in ChromeOS)
// to androidPath (in Android) using adb, and then checks whether the file can
// be accessed under crosPath (in ChromeOS).
func testPushToARCAndReadFromCros(ctx context.Context, a *arc.ARC, sourcePath, androidPath, crosPath string) (retErr error) {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	expected, err := os.ReadFile(sourcePath)
	if err != nil {
		return errors.Wrapf(err, "failed to read from %s in ChromeOS", sourcePath)
	}

	if err := a.WriteFile(ctx, androidPath, expected); err != nil {
		return errors.Wrapf(err, "failed to write to %s in Android", androidPath)
	}
	defer func(ctx context.Context) {
		if err := a.RemoveAll(ctx, androidPath); err != nil {
			if retErr == nil {
				retErr = errors.Wrapf(err, "failed remove %s in Android", androidPath)
			} else {
				testing.ContextLogf(ctx, "Failed to remove %s in Android: %v", androidPath, err)
			}
		}
	}(cleanupCtx)

	actual, err := os.ReadFile(crosPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read from %s in ChromeOS", crosPath)
	}
	if !bytes.Equal(actual, expected) {
		return errors.Errorf("content mismatch between %s in Android and %s in ChromeOS", androidPath, crosPath)
	}

	return nil
}

// testFilesAppIntegrationForMyFiles checks whether a file put in the ChromeOS MyFiles directory
// can be:
//   - edited with test Android app after it is opened with the app from Files app's "Open with..."
//     menu, and
//   - opened with test Android app via SAF.
func testFilesAppIntegrationForMyFiles(ctx context.Context, a *arc.ARC, cr *chrome.Chrome, d *ui.Device, myFilesPath, outDir string) error {
	const (
		filename    = "storage.txt"
		fileContent = "this is a test"
	)

	testing.ContextLog(ctx, "Testing Files app integration")

	testFilePath := filepath.Join(myFilesPath, filename)
	if err := os.WriteFile(testFilePath, []byte(fileContent), 0666); err != nil {
		return errors.Wrapf(err, "failed to create test file %s", testFilePath)
	}
	defer os.Remove(testFilePath)
	// The test file's mode will be 0644 due to umask. Change the ownership of the file
	// appropriately so that the test Android app can write to it.
	if err := os.Chown(testFilePath, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return errors.Wrapf(err, "failed to chown test file %s", testFilePath)
	}

	config := storage.TestConfig{
		DirName:     filesapp.MyFiles,
		FileName:    filename,
		FileContent: fileContent,
		OutDir:      outDir,
		ReadOnly:    false,
	}
	return storage.TestFilesAppIntegration(ctx, a, cr, d, config)
}
