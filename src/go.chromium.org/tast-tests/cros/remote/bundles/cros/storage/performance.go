// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/bounds"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
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
		Timeout:      10 * time.Minute,
		Attr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
		Params: []testing.Param{{
			Name: "0000_nvme_seq_write",
			Val: perfTestCase{
				DataPath: "seq_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_write_write_bw.*`),
					Bounds: bounds.Min(102_400), // KiB/sec, 100 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageSeqWriteTp, tdreq.NvmeStorageSeqWriteLatency},
		}, {
			Name: "0001_nvme_seq_read",
			Val: perfTestCase{
				DataPath: "seq_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_read_read_bw.*`),
					Bounds: bounds.Min(256_000), // KiB/sec, 250 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageSeqReadTp, tdreq.NvmeStorageSeqReadLatency},
		}, {
			Name: "0000_emmc_seq_write",
			Val: perfTestCase{
				DataPath: "seq_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_write_write_bw.*`),
					Bounds: bounds.Min(20_480), // KiB/sec, 20 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorageSeqWriteTp, tdreq.EmmcStorageSeqWriteLatency},
		}, {
			Name: "0001_emmc_seq_read",
			Val: perfTestCase{
				DataPath: "seq_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_read_read_bw.*`),
					Bounds: bounds.Min(51_200), // KiB/sec, 50 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorageSeqReadTp, tdreq.EmmcStorageSeqReadLatency},
		}, {
			Name: "0000_ufs_seq_write",
			Val: perfTestCase{
				DataPath: "seq_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_write_write_bw.*`),
					Bounds: bounds.Min(102_400), // KiB/sec, 100 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageSeqWriteTp, tdreq.UfsStorageSeqWriteLatency},
		}, {
			Name: "0001_ufs_seq_read",
			Val: perfTestCase{
				DataPath: "seq_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_seq_read_read_bw.*`),
					Bounds: bounds.Min(256_000), // KiB/sec, 250 MiB/sec
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageSeqReadTp, tdreq.UfsStorageSeqReadLatency},
		}, {
			Name: "0002_16k_write_iops",
			Val: perfTestCase{
				DataPath: "16k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_16k_write_write_iops.*`),
					Bounds: bounds.Min(150), // iops
				}},
			},
			ExtraRequirements: []string{tdreq.Storage16kWriteIOPs},
		}, {
			Name: "0003_16k_read_iops",
			Val: perfTestCase{
				DataPath: "16k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_16k_read_read_iops.*`),
					Bounds: bounds.Min(1500), // iops
				}},
			},
			ExtraRequirements: []string{tdreq.Storage16kReadIOPs},
		}, {
			Name: "0004_nvme_16k_write",
			Val: perfTestCase{
				DataPath: "16k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage16kWriteLatency},
		}, {
			Name: "0005_nvme_16k_read",
			Val: perfTestCase{
				DataPath: "16k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage16kReadLatency},
		}, {
			Name: "0004_emmc_16k_write",
			Val: perfTestCase{
				DataPath: "16k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage16kWriteLatency},
		}, {
			Name: "0005_emmc_16k_read",
			Val: perfTestCase{
				DataPath: "16k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage16kReadLatency},
		}, {
			Name: "0004_ufs_16k_write",
			Val: perfTestCase{
				DataPath: "16k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage16kWriteLatency},
		}, {
			Name: "0005_ufs_16k_read",
			Val: perfTestCase{
				DataPath: "16k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage16kReadLatency},
		}, {
			Name: "0006_nvme_4k_write",
			Val: perfTestCase{
				DataPath: "4k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kWriteLatency},
		}, {
			Name: "0007_nvme_4k_read",
			Val: perfTestCase{
				DataPath: "4k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kReadLatency},
		}, {
			Name: "0006_emmc_4k_write",
			Val: perfTestCase{
				DataPath: "4k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kWriteLatency},
		}, {
			Name: "0007_emmc_4k_read",
			Val: perfTestCase{
				DataPath: "4k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kReadLatency},
		}, {
			Name: "0006_ufs_4k_write",
			Val: perfTestCase{
				DataPath: "4k_write",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kWriteLatency},
		}, {
			Name: "0007_ufs_4k_read",
			Val: perfTestCase{
				DataPath: "4k_read",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kReadLatency},
		}, {
			Name: "0008_nvme_4k_write_qd4",
			Val: perfTestCase{
				DataPath: "4k_write_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kQD4WriteLatency},
		}, {
			Name: "0009_nvme_4k_read_qd4",
			Val: perfTestCase{
				DataPath: "4k_read_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorage4kQD4ReadLatency},
		}, {
			Name: "0008_emmc_4k_write_qd4",
			Val: perfTestCase{
				DataPath: "4k_write_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kQD4WriteLatency},
		}, {
			Name: "0009_emmc_4k_read_qd4",
			Val: perfTestCase{
				DataPath: "4k_read_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorage4kQD4ReadLatency},
		}, {
			Name: "0008_ufs_4k_write_qd4",
			Val: perfTestCase{
				DataPath: "4k_write_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_write_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kQD4WriteLatency},
		}, {
			Name: "0009_ufs_4k_read_qd4",
			Val: perfTestCase{
				DataPath: "4k_read_qd4",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorage4kQD4ReadLatency},
		}, {
			Name: "0010_nvme_surfing",
			Val: perfTestCase{
				DataPath: "surfing",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
			ExtraRequirements: []string{tdreq.NvmeStorageUserSimReadLatency, tdreq.NvmeStorageUserSimWriteLatency},
		}, {
			Name: "0010_emmc_surfing",
			Val: perfTestCase{
				DataPath: "surfing",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(35_000_000), // 35 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.EmmcOrBridge()),
			ExtraRequirements: []string{tdreq.EmmcStorageUserSimReadLatency, tdreq.EmmcStorageUserSimWriteLatency},
		}, {
			Name: "0010_ufs_surfing",
			Val: perfTestCase{
				DataPath: "surfing",
				Bounds: []bounds.MetricBounds{{
					Metric: bounds.MatchRegexp(`.*_read_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}, {
					Metric: bounds.MatchRegexp(`.*_write_clat_ns_percentile_99.000000.*`),
					Bounds: bounds.Max(12_000_000), // 12 ms
				}},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
			ExtraRequirements: []string{tdreq.UfsStorageUserSimReadLatency, tdreq.UfsStorageUserSimWriteLatency},
		}},
	})
}

type perfTestCase struct {
	DataPath string
	Bounds   []bounds.MetricBounds
}

func Performance(ctx context.Context, s *testing.State) {
	val := s.Param().(perfTestCase)
	defer util.FatalIfBoundsCheckFail(ctx, val.Bounds, s)

	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	err = util.WriteAVLInfo(ctx, disk, s.OutDir())
	if err != nil {
		s.Fatal("Failed to write AVL info: ", err)
	}

	err = util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk).
		WithSize(1024*1024*1024).
		WithRunTimeSec(300).
		WithJobFromFile(s.DataPath(val.DataPath)).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}
}
