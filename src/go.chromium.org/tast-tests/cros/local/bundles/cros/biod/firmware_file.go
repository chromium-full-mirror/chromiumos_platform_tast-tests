// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package biod

import (
	"context"
	"path/filepath"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FirmwareFile,
		Desc: "Checks that the specific fingerprint firmware file is in rootfs",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"hesling@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:fingerprint-cq"},
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
	})
}

// FirmwareFile checks that the specific fingerprint firmware file is in rootfs.
func FirmwareFile(ctx context.Context, s *testing.State) {
	board, err := crosconfig.Get(ctx, "/fingerprint", "board")
	if err != nil {
		s.Fatal("Failed to fetch board from cros-config: ", err)
	}

	files, err := filepath.Glob(fp.FirmwareFilePattern(fp.BoardName(board)))
	if err != nil {
		s.Fatal("Failed to glob for firmware files: ", err)
	}
	if len(files) == 0 {
		s.Fatalf("Couldn't find the fingerprint firmware file for board %q", board)
	}
	if len(files) > 1 {
		s.Fatal("Too many fingerprint firmware files were found: ", files)
	}
	s.Logf("Found firmware file %q", files[0])
}
