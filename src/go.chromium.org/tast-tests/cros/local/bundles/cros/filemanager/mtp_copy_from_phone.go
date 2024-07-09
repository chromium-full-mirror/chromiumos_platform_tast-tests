// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/local/chrome/mtp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MTPCopyFromPhone,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify it is possible to copy files from the phone via MTP",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"mattlui@google.com",
		},
		Attr:         []string{"group:mtp"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      3 * time.Minute,
		Fixture:      "mtpWithAndroid",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-ac496e62-db9a-4ae2-9c4a-d322f6d62b84",
			},
		},
	})
}

func MTPCopyFromPhone(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*mtp.FixtData).Chrome
	tconn := s.FixtValue().(*mtp.FixtData).TestConn
	adb := s.FixtValue().(*mtp.FixtData).AdbDevice

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	const testFile = "MTPCopyFromPhone.txt"
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's MyFiles path: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	tempFileLocation := filepath.Join(myFilesPath, testFile)
	originalFileLocation := filepath.Join(android.DownloadDir, testFile)
	copiedFileLocation := filepath.Join(downloadsPath, testFile)

	if err := ioutil.WriteFile(tempFileLocation, []byte("blahblah"), 0644); err != nil {
		s.Fatalf("Failed to create file %q: %s", tempFileLocation, err)
	}
	defer os.Remove(tempFileLocation)

	if err := adb.PushFile(ctx, tempFileLocation, android.DownloadDir); err != nil {
		s.Fatal("Failed to push file to MTP: ", err)
	}
	defer adb.RemoveContents(cleanupCtx, android.DownloadDir)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "MTPCopyFromPhone")

	if err := uiauto.Combine("Copy a test file from the phone to the downloads directory",
		files.OpenPath(filesapp.FilesTitlePrefix+mtp.DeviceName, mtp.DeviceName, "Download"),
		files.CopyFileToClipboard(testFile),
		files.OpenDownloads(),
		files.PasteFileFromClipboard(kb),
		files.WaitForFile(testFile),
	)(ctx); err != nil {
		s.Fatalf("Failed to copy %q from the phone: %v", originalFileLocation, err)
	}
	defer os.Remove(copiedFileLocation)

	original, err := adb.ReadFile(ctx, originalFileLocation)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", originalFileLocation, err)
	}

	copied, err := ioutil.ReadFile(copiedFileLocation)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", copiedFileLocation, err)
	}

	if !bytes.Equal(copied, original) {
		s.Fatalf("Content mismatch between the original and copied files: got %q; want %q", copied, original)
	}
}
