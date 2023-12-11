// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type arcManyFilesTestParams struct {
	crosPath    func(ctx context.Context, username string) (string, error)
	androidPath string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManyFiles,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies ARCVM storage integration works with a large number of files",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBooted",
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{{
			Name:      "downloads",
			ExtraAttr: []string{"informational"},
			Val: arcManyFilesTestParams{
				crosPath:    cryptohome.DownloadsPath,
				androidPath: "/storage/emulated/0/Download",
			},
		}, {
			Name:      "myfiles",
			ExtraAttr: []string{"informational"},
			Val: arcManyFilesTestParams{
				crosPath:    cryptohome.MyFilesPath,
				androidPath: "/storage/" + arc.MyFilesUUID,
			},
		}},
	})
}

func ManyFiles(ctx context.Context, s *testing.State) {
	const (
		// This should be kept in sync with MainActivity.java in ArcManyFilesTest.
		numberOfFiles = 10000

		apkName       = "ArcManyFilesTest.apk"
		pkgName       = "org.chromium.arc.testapp.manyfiles"
		activityName  = pkgName + ".MainActivity"
		resultLabelID = pkgName + ":id/result"
		targetDirName = "many_files_test"
		fileContent   = "this is a test"
	)

	params := s.Param().(arcManyFilesTestParams)

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	// Set up a directory to store the test files.
	crosPath, err := params.crosPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get CrOS-side path")
	}
	targetDir := filepath.Join(crosPath, targetDirName)
	if err := os.Mkdir(targetDir, 0755); err != nil {
		s.Fatal("Failed to create target directory: ", err)
	}
	defer os.RemoveAll(targetDir)
	if err := os.Chown(targetDir, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		s.Fatal("Failed to chown target directory: ", err)
	}

	// Create a lot of text files in the target directory from the host side.
	for i := 0; i < numberOfFiles; i++ {
		tpath := filepath.Join(targetDir, fmt.Sprintf("storage_%d", i))
		if err := ioutil.WriteFile(tpath, []byte(fileContent), 0644); err != nil {
			s.Fatal("Failed to write a data file: ", err)
		}
		if (i+1)%(numberOfFiles/10) == 0 {
			s.Logf("Created %d files in %s", i+1, targetDir)
		}
	}

	s.Log("Installing " + apkName)
	if err := a.Install(ctx, arc.APKPath(apkName)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	// Allow the app to create files anywhere in external storage.
	if err := a.Command(ctx, "appops", "set", pkgName, "MANAGE_EXTERNAL_STORAGE", "allow").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed granting all files access permission: ", err)
	}

	s.Log("Starting MainActivity")
	act, err := arc.NewActivity(a, pkgName, activityName)
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	opts := []arc.ActivityStartOption{
		arc.WithWaitForLaunch(),
		arc.WithExtraString("path", filepath.Join(params.androidPath, targetDirName)),
	}
	if err := act.Start(ctx, tconn, opts...); err != nil {
		s.Fatal("Failed to start MainActivity: ", err)
	}

	if err := verifyTestResult(ctx, d, targetDir, resultLabelID); err != nil {
		s.Fatal("Failed to check the test result: ", err)
	}
}

func verifyTestResult(ctx context.Context, d *ui.Device, targetDir, labelID string) error {
	// Keep these in sync with MainActivity.java of ArcManyFilesTest.
	const (
		fileNameCreatedByApp       = "storage.txt"
		appSideCheckSuccessMessage = "PASS"
	)

	uiObj := d.Object(ui.ID(labelID))
	if err := uiObj.WaitForText(ctx, appSideCheckSuccessMessage, 2*time.Minute); err != nil {
		return errors.Wrapf(err, "failed to find the label id %s", labelID)
	}

	filePathCreatedByAndroid := filepath.Join(targetDir, fileNameCreatedByApp)
	if _, err := os.Stat(filePathCreatedByAndroid); err != nil {
		return errors.Wrapf(err, "failed to check the existence of %s", filePathCreatedByAndroid)
	}
	return nil
}
