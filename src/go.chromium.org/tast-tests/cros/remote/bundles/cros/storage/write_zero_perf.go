// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strconv"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WriteZeroPerf,
		Desc: "Verifies Write Zero is not slower than DD",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Requirements: []string{
			tdreq.NvmeBlkZeroOutPerf,
		},
	})
}

func WriteZeroPerf(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	ddBsCount := strconv.FormatInt(int64(disk.Size)/512/1024, 10)
	startDD := time.Now()
	if _, err := util.RunCmdWithOutput(ctx, s.DUT(), "dd", "if=/dev/zero", "of="+disk.Path, "bs=512k", "count="+ddBsCount, "oflag=direct"); err != nil {
		s.Fatal("Failed to run dd: ", err)
	}
	if _, err := util.RunCmdWithOutput(ctx, s.DUT(), "sync", disk.Path); err != nil {
		s.Fatal("Failed to run sync after dd: ", err)
	}
	ddnsec := time.Since(startDD).Milliseconds()
	wznsecLim := ddnsec * 105 / 100 // 5% tolerance

	startWZ := time.Now()
	if _, err := util.RunCmdWithOutput(ctx, s.DUT(), "blkdiscard", "-f", "-z", disk.Path); err != nil {
		s.Fatal("Failed to run zerout: ", err)
	}
	wznsec := time.Since(startWZ).Milliseconds()

	if wznsec > wznsecLim {
		s.Fatalf("Zero-out is more than 5 percent slower than overwrite: %d > %d", wznsec, wznsecLim)
	}
}
