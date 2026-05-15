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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Fsp,
		Desc:     "Android app can read files on pseudo file systems using File System Provider (FSP) via FilesApp",
		Contacts: []string{"arc-storage@google.com", "youkichihosoi@chromium.org", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Data:         []string{"fsp_extension/manifest.json", "fsp_extension/service-worker.js"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			},
		},
	})
}

// Fsp implements the test scenario of arc.Fsp.
func Fsp(ctx context.Context, s *testing.State) {
	const (
		fspMountName = "FSPTastExt"
		// These need to match with the content under |fspMountName|.
		filename    = "roses.txt"
		fileContent = "Roses are red."
	)

	cr, err := chrome.New(
		ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.UnpackedExtension(filepath.Dir(s.DataPath("fsp_extension/manifest.json"))),
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

	config := storage.TestConfig{
		DirName:     fspMountName,
		FileName:    filename,
		FileContent: fileContent,
		OutDir:      s.OutDir(),
		ReadOnly:    true,
	}
	if err := storage.TestFilesAppIntegration(ctx, a, cr, d, config); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
