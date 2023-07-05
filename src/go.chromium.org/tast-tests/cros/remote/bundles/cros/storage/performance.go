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
		LacrosStatus: testing.LacrosVariantUnneeded,
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
		}, {
			Name:              "nvme_4k_read",
			Val:               "4k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kReadLatency},
		}, {
			Name:              "nvme_4k_write",
			Val:               "4k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kWriteLatency},
		}, {
			Name:              "emmc_4k_read",
			Val:               "4k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kReadLatency},
		}, {
			Name:              "emmc_4k_write",
			Val:               "4k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kWriteLatency},
		}, {
			Name:              "ufs_4k_read",
			Val:               "4k_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kReadLatency},
		}, {
			Name:              "ufs_4k_write",
			Val:               "4k_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kWriteLatency},
		}, {
			Name:              "nvme_4k_read_qd4",
			Val:               "4k_read_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kQD4ReadLatency},
		}, {
			Name:              "nvme_4k_write_qd4",
			Val:               "4k_write_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kQD4WriteLatency},
		}, {
			Name:              "emmc_4k_read_qd4",
			Val:               "4k_read_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kQD4ReadLatency},
		}, {
			Name:              "emmc_4k_write_qd4",
			Val:               "4k_write_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kQD4WriteLatency},
		}, {
			Name:              "ufs_4k_read_qd4",
			Val:               "4k_read_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kQD4ReadLatency},
		}, {
			Name:              "ufs_4k_write_qd4",
			Val:               "4k_write_qd4",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kQD4WriteLatency},
		}, {
			Name:              "nvme_seq_read",
			Val:               "seq_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageSeqReadTp, tdreq.NvmeStorageSeqReadLatency},
		}, {
			Name:              "nvme_seq_write",
			Val:               "seq_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageSeqWriteTp, tdreq.NvmeStorageSeqWriteLatency},
		}, {
			Name:              "emmc_seq_read",
			Val:               "seq_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorageSeqReadTp, tdreq.EmmcStorageSeqReadLatency},
		}, {
			Name:              "emmc_seq_write",
			Val:               "seq_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorageSeqWriteTp, tdreq.EmmcStorageSeqWriteLatency},
		}, {
			Name:              "ufs_seq_read",
			Val:               "seq_read",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageSeqReadTp, tdreq.UfsStorageSeqReadLatency},
		}, {
			Name:              "ufs_seq_write",
			Val:               "seq_write",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageSeqWriteTp, tdreq.UfsStorageSeqWriteLatency},
		}, {
			Name:              "nvme_surfing",
			Val:               "surfing",
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageUserSimReadLatency, tdreq.NvmeStorageUserSimWriteLatency},
		}, {
			Name:              "emmc_surfing",
			Val:               "surfing",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			ExtraRequirements: []string{tdreq.EmmcStorageUserSimReadLatency, tdreq.EmmcStorageUserSimWriteLatency},
		}, {
			Name:              "ufs_surfing",
			Val:               "surfing",
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageUserSimReadLatency, tdreq.UfsStorageUserSimWriteLatency},
		}},
	})
}

func Performance(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
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
