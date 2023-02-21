// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/common/android"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/arc/storage"
	"chromiumos/tast/local/chrome/mtp"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/testing"
)

// mtpURIPrefix is the expected prefix of the Content URI for the Android
// device under test. The full URI would contain the device's serial number,
// which would be different for different devices.
const (
	mtpURIPrefix = "content://org.chromium.arc.chromecontentprovider/externalfile%3Afileman-mtp-mtp"
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
		Timeout:      5 * time.Minute,
		Fixture:      "mtpWithAndroid",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_p"},
			}, {
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
	})
}

func MTP(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*mtp.FixtData).Chrome
	tconn := s.FixtValue().(*mtp.FixtData).TestConn
	adb := s.FixtValue().(*mtp.FixtData).AdbDevice

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a, err := arc.New(ctx, s.OutDir())
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
	const textFile = "storage.txt"
	testFileLocation := filepath.Join(downloadsPath, textFile)
	if err := ioutil.WriteFile(testFileLocation, []byte("this is a test"), 0777); err != nil {
		s.Fatalf("Creating file %s failed: %s", testFileLocation, err)
	}
	defer os.Remove(testFileLocation)

	if err := adb.PushFile(ctx, testFileLocation, android.DownloadDir); err != nil {
		s.Fatal("Failed to push file to MTP: ", err)
	}
	defer adb.RemoveContents(cleanupCtx, android.DownloadDir)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	config := storage.TestConfig{DirName: mtp.DeviceName, DirTitle: filesapp.FilesTitlePrefix + mtp.DeviceName,
		SubDirectories: []string{"Download"}, FileName: textFile}
	expectations := []storage.Expectation{
		{LabelID: storage.ActionID, Value: storage.ExpectedAction},
		{LabelID: storage.URIID, Predicate: func(actual string) bool {
			return strings.HasPrefix(actual, mtpURIPrefix) &&
				strings.HasSuffix(actual, "%2FDownload%2Fstorage.txt")
		}},
		{LabelID: storage.FileContentID, Value: storage.ExpectedFileContent}}

	if err := storage.TestOpenWithAndroidApp(ctx, a, cr, d, config, expectations); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
