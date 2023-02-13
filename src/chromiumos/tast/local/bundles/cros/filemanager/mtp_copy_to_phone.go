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

	"chromiumos/tast/common/android"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome/mtp"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MTPCopyToPhone,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify it is possible to copy files to the phone via MTP",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"ashpakov@google.com",
		},
		Attr:         []string{"group:mtp"},
		SoftwareDeps: []string{"chrome"},
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

func MTPCopyToPhone(ctx context.Context, s *testing.State) {
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
	defer kb.Close()

	const testFile = "MTPCopyToPhone.txt"
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	originalFileLocation := filepath.Join(downloadsPath, testFile)
	copiedFileLocation := filepath.Join(android.DownloadDir, testFile)

	if err := ioutil.WriteFile(originalFileLocation, []byte("blahblah"), 0644); err != nil {
		s.Fatalf("Failed to create file %q: %s", originalFileLocation, err)
	}
	defer os.Remove(originalFileLocation)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "MTPCopyToPhone")

	refreshButton := nodewith.Name("Refresh").Role(role.Button)
	if err := uiauto.Combine("Copy a test file from the downloads directory to the phone",
		files.OpenDownloads(),
		files.CopyFileToClipboard(testFile),
		files.OpenPath(filesapp.FilesTitlePrefix+mtp.DeviceName, mtp.DeviceName, "Download"),
		files.LeftClick(refreshButton), // TODO: remove once b/269057578 is fixed.
		files.PasteFileFromClipboard(kb),
		files.LeftClickUntil(
			refreshButton, // TODO: remove once b/269058614 is fixed.
			files.WithTimeout(time.Second).WaitForFile(testFile),
		),
	)(ctx); err != nil {
		s.Fatalf("Failed to copy %q to the phone: %v", originalFileLocation, err)
	}
	defer adb.RemoveContents(cleanupCtx, android.DownloadDir)

	original, err := ioutil.ReadFile(originalFileLocation)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", originalFileLocation, err)
	}

	copied, err := adb.ReadFile(ctx, copiedFileLocation)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", copiedFileLocation, err)
	}

	if !bytes.Equal(copied, original) {
		s.Fatalf("Content mismatch between the original and copied files: got %q; want %q", copied, original)
	}
}
