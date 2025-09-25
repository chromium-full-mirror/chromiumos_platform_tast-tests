// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"bytes"
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PassthroughDLCDev,
		Desc: "Checks for passthrough files for DLC and dev image",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

var testPaths = []string{"/mnt/stateful_partition/unencrypted/dev_image.block"}

func PassthroughDLCDev(ctx context.Context, s *testing.State) {
	d := s.DUT()
	hasDefaultKey := util.HasDefaultKeyStatefulDiskLayout(ctx, d)
	if !hasDefaultKey {
		s.Log("Skipping test: DUT does not have default-key-stateful disk layout")
		return
	}

	// Get stateful partition.
	statefulDev, err := util.GetStatefulPartition(ctx, d)
	if err != nil {
		s.Fatal("Failed to get stateful partition: ", err)
	}

	// Finding all dlc.img under /var/cache/dlc/<>/package/dlc_a/dlc.img.

	findCmd := `find /var/cache/dlc -path "*/package/dlc_a/dlc.img" -type f`
	findOutput, err := d.Conn().CommandContext(ctx, "sh", "-c", findCmd).Output()
	if err != nil {
		s.Fatal("Failed to find dlc.img files: ", err)
	}
	dlcPaths := strings.Split(strings.TrimSpace(string(findOutput)), "\n")
	testPaths = append(testPaths, dlcPaths...)

	// Iterating for all testPaths.
	for _, path := range testPaths {

		skipsize, err := util.GetSkipSize(ctx, d, path)
		if err != nil {
			s.Fatal("Failed to get physical skipsize: ", err)
		}

		// Getting File Hash.
		fileHash, err := util.GetMD5(ctx, d, path, 0)
		if err != nil {
			s.Fatal("Failed to get filehash: ", err)
		}

		// Getting Disk Hash.
		diskHash, err := util.GetMD5(ctx, d, statefulDev, skipsize)
		if err != nil {
			s.Fatal("Failed to get filehash: ", err)
		}

		// Comparing both Hashes.
		if !bytes.Equal(fileHash[:], diskHash[:]) {
			s.Errorf("Hash mismatch for %s: file (%s) vs disk (%s)", path, fileHash, diskHash)
		}

	}

}
