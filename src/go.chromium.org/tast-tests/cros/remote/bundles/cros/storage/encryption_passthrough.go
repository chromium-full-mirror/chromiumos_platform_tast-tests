// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"bytes"
	"context"
	"encoding/hex"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"

	"go.chromium.org/tast/core/testing"
)

const (
	filePath = "/mnt/stateful_partition/unencrypted/"
	fileName = "test_file"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: EncryptionPassthrough,
		Desc: "Validation test for encryption passthrough",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func EncryptionPassthrough(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	hasDefaultKey := util.HasDefaultKeyStatefulDiskLayout(ctx, dut)
	if !hasDefaultKey {
		s.Log("Skipping test: DUT does not have default-key-stateful disk layout")
		return
	}

	// This is to create test_file file.
	_, err := util.DDCommand(ctx, dut, "/dev/random", filePath+fileName, "1M", 32, 0)
	if err != nil {
		s.Fatal("Failed to create a file: ", err)
	}

	// It will sync to force the extents to disk.
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", "sync")
	if err != nil {
		s.Fatal("Failed to sync: ", err)
	}

	skipSize, err := util.GetSkipSize(ctx, dut, filePath+fileName)
	if err != nil {
		s.Fatal("Error while trying to get the skip size: ", err)
	}

	// This function will return the stateful partition path.
	statefulDev, err := util.GetStatefulPartition(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get statefulDev: ", err)
	}

	fileHash, err := util.GetMD5(ctx, dut, filePath+fileName, 0)
	if err != nil {
		s.Fatal("Failed to get hash for file: ", err)
	}

	blockHash, err := util.GetMD5(ctx, dut, statefulDev, skipSize)
	if err != nil {
		s.Fatal("Failed to get hash for block: ", err)
	}
	// This will compare the hashes of file and disk.
	if !bytes.Equal(fileHash[:], blockHash[:]) {
		s.Fatalf("Hash mismatch: file=%s, block=%s", hex.EncodeToString(fileHash[:]), hex.EncodeToString(blockHash[:]))
	}
}
