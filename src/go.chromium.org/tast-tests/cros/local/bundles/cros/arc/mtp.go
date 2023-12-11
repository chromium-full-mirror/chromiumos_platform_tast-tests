// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/storage"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/mtp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// arc.Mtp / arc.Mtp.vm tast tests depend on the use of actual Android device in the lab.
// As part of the test, a file will be pushed and read from it. Therefore, these tests have
// the following constraints:
// 1. It can only be run on a special lab setup.
// 2. The device folder names etc being used are hard-coded for the setup.

func init() {
	testing.AddTest(&testing.Test{
		Func:         MTP,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "ARC++/ARCVM Android app can read files on external Android device (with MTP) via FilesApp",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mtp"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
		Fixture:      "mtpWithAndroid",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			},
		},
	})
}

// MTP implements the test scenario of arc.MTP.
func MTP(ctx context.Context, s *testing.State) {
	const (
		filename    = "storage.txt"
		fileContent = "this is a test"
	)

	cr := s.FixtValue().(*mtp.FixtData).Chrome
	tconn := s.FixtValue().(*mtp.FixtData).TestConn
	adb := s.FixtValue().(*mtp.FixtData).AdbDevice

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}

	// Set up the test file.
	testFileLocation := filepath.Join(downloadsPath, filename)
	if err := ioutil.WriteFile(testFileLocation, []byte(fileContent), 0777); err != nil {
		s.Fatalf("Creating file %s failed: %s", testFileLocation, err)
	}
	defer os.Remove(testFileLocation)

	if err := adb.PushFile(ctx, testFileLocation, android.DownloadDir); err != nil {
		s.Fatal("Failed to push file to MTP: ", err)
	}
	defer adb.RemoveContents(cleanupCtx, android.DownloadDir)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	config := storage.TestConfig{
		DirName:        mtp.DeviceName,
		SubDirectories: []string{"Download"},
		FileName:       filename,
		FileContent:    fileContent,
		OutDir:         s.OutDir(),
		ReadOnly:       true,
	}
	if err := storage.TestFilesAppIntegration(ctx, a, cr, d, config); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
