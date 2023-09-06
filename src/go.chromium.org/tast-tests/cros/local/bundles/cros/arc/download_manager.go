// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"encoding/binary"
	"io/ioutil"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcdownload"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DownloadManager,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks whether ARC can download files through DownloadManager",
		Contacts: []string{"arc-storage@google.com",
			"youkichihosoi@chromium.org",
			"momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Data:         []string{"capybara.jpg"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 7 * time.Minute,
	})
}

func DownloadManager(ctx context.Context, s *testing.State) {
	const (
		filename   = "capybara.jpg"
		targetPath = "/storage/emulated/0/Download/" + filename
	)

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice
	cr := s.FixtValue().(*arc.PreData).Chrome
	sourcePath := s.DataPath(filename)

	arcdownload.DownloadTestFile(ctx, cr, a, d, sourcePath, targetPath)

	defer func(ctx context.Context) {
		if err := a.RemoveAll(ctx, targetPath); err != nil {
			s.Fatalf("Failed to remove %s: %v", targetPath, err)
		}
	}(ctx)

	// Check whether the downloaded file is accessible from ChromeOS.
	original, err := ioutil.ReadFile(sourcePath)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", sourcePath, err)
	}

	cryptohomeUserPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatalf("Failed to get the cryptohome user path for %s: %v",
			cr.NormalizedUser(), err)
	}
	targetPathInCros := filepath.Join(cryptohomeUserPath, "MyFiles", "Downloads",
		filename)

	downloaded, err := ioutil.ReadFile(targetPathInCros)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", targetPathInCros, err)
	}

	if !bytes.Equal(downloaded, original) {
		s.Fatalf("Content mismatch between the original file (%d bytes) and the downloaded file (%d bytes)", binary.Size(original), binary.Size(downloaded))
	}
}
