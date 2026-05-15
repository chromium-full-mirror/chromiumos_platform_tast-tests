// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/storage"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Drivefs,
		Desc:     "Android app can read files on Drive FS (Google Drive) via FilesApp",
		Contacts: []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "group:arc-functional", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "drivefs", "gaia"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		VarDeps:      []string{arcCommon.DrivefsPoolVarName},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
	})
}

// Drivefs implements the test scenario of arc.Drivefs.
func Drivefs(ctx context.Context, s *testing.State) {
	const (
		filename    = "storage_drivefs.txt"
		fileContent = "this is a test"
	)

	cr, err := chrome.New(
		ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.GAIALoginPool(dma.CredsFromPool(arcCommon.DrivefsPoolVarName)),
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

	mountPath, err := drivefs.WaitForDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed waiting for DriveFS to start: ", err)
	}
	drivefsRoot := path.Join(mountPath, "root")

	// Since the test account can be shared with other concurrently running
	// tests, create the test file only when it's missing, and do not remove it
	// after the test (b/179876719).
	testFilePath := filepath.Join(drivefsRoot, filename)
	if _, err := os.Stat(testFilePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(testFilePath, []byte(fileContent), 0666); err != nil {
			s.Fatalf("Failed to create test file %s: %v", testFilePath, err)
		}
	}

	config := storage.TestConfig{
		DirName:       filesapp.GoogleDrive,
		FileName:      filename,
		DirTitle:      filesapp.MyDrive,
		FileContent:   fileContent,
		OutDir:        s.OutDir(),
		CheckFileType: true,
		ReadOnly:      true,
	}
	if err := storage.TestFilesAppIntegration(ctx, a, cr, d, config); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
