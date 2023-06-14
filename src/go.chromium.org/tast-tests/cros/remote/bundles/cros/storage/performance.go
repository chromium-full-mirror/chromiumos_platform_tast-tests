// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Performance,
		Desc: "Measure performance of NVMe storage",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.USBDevModeWithReinstall,
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			Name:              "nvme_16k_read",
			Val:               "16k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.Storage16kReadIOPs, tdreq.NvmeStorage16kReadLatency},
		}, {
			Name:              "nvme_16k_write",
			Val:               "16k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.Storage16kWriteIOPs, tdreq.NvmeStorage16kWriteLatency},
		}, {
			Name:              "emmc_16k_read",
			Val:               "16k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.Storage16kReadIOPs, tdreq.EmmcStorage16kReadLatency},
		}, {
			Name:              "emmc_16k_write",
			Val:               "16k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.Storage16kWriteIOPs, tdreq.EmmcStorage16kWriteLatency},
		}, {
			Name:              "ufs_16k_read",
			Val:               "16k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.Storage16kReadIOPs, tdreq.UfsStorage16kReadLatency},
		}, {
			Name:              "ufs_16k_write",
			Val:               "16k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.Storage16kWriteIOPs, tdreq.UfsStorage16kWriteLatency},
		}},
	})
}

func Performance(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetInternalStorageFromRemovableBoot(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	err = util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk).
		WithRunTimeSec(300).
		WithJobFromFile(s.DataPath(s.Param().(string))).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}
}
