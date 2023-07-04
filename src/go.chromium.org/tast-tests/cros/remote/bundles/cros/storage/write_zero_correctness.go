// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WriteZeroCorrectness,
		Desc: "Verifies Write Zero leaves device zeroed-out",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		//TODO(dlunev): add requirements for link with PVS, once added.
	})
}

func WriteZeroCorrectness(ctx context.Context, s *testing.State) {
	disk, err := util.GetStandbyRootfsFromInternalBoot(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	err = util.TestConfig{}.
		WithDisk(disk).
		WithJobFromFile(s.DataPath("tbw_probe")).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	if _, err := util.RunCmdWithOutput(ctx, s.DUT(), "blkdiscard", "-f", "-z", disk.Path); err != nil {
		s.Fatal("Failed to run zerout: ", err)
	}

	ddBsCount := strconv.FormatInt(int64(disk.Size)/512/1024, 10)

	// Preformat the command so we can use pipe
	cmd := "dd if=" + disk.Path + " iflag=direct bs=512k count=" + ddBsCount + " | cmp - /dev/zero 2>&1"

	// Ignore the error for cmp with complain about EOF.
	out, _ := util.RunCmdWithStringOutput(ctx, s.DUT(), "bash", "-c", cmd)

	if strings.Contains(out, "differ") {
		s.Fatal("Device not-zeroed: ", out)
	}

	if !strings.Contains(out, "cmp: EOF on -") {
		s.Fatal("Unexpected comparison termination: ", out)
	}
}
