// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const fileName = "files_app_chrome_dino.pdf"

func init() {
	testing.AddTest(&testing.Test{
		Func:         FilesApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that files app is working with managed guest sessions by saving and opening a pdf",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"bfranz@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture: fixture.FakeDMSEnrolled,
		Data:    []string{fileName},
	})
}

// FilesApp tests the Files App by:
// 1. Starting a managed guest session (MGS)
// 2. Copying the pdf file to Downloads
// 3. Launching and using the Files App to open the PDF file
// 4. Verifying the file is opened by checking the Gallery app is opened
func FilesApp(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start MGS: ", err)
	}
	defer mgs.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	// Clean up in the end.
	defer func() {
		if err := removeAllFilesInDirectory(downloadsPath); err != nil {
			s.Error("Failed to clean up Downloads: ", err)
		}
	}()

	if err := fsutil.CopyFile(
		s.DataPath(fileName), filepath.Join(downloadsPath, fileName),
	); err != nil {
		s.Fatal("Failed to copy file to Downloads: ", err)
	}

	fa, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch files app: ", err)
	}

	if err := uiauto.Combine("open filesapp and look at file",
		fa.OpenDownloads(),
		fa.WaitForFile(fileName),
		fa.OpenFile(fileName),
	)(ctx); err != nil {
		s.Fatal("Failed to verify file exists: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Second*30); err != nil {
		s.Fatal("Failed to check Gallery in shelf: ", err)
	}
}

// removeAllFilesInDirectory removes all files in a directory.
// TODO(b/268604942): Move to a common package and reuse it in tests.
func removeAllFilesInDirectory(directory string) error {
	files, err := os.ReadDir(directory)
	if err != nil {
		return errors.Wrapf(err, "failed to read files in %s", directory)
	}
	for _, f := range files {
		path := filepath.Join(directory, f.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to RemoveAll(%q)", path)
		}
	}
	return nil
}
