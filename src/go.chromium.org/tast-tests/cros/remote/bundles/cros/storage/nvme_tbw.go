// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NvmeTBW,
		Desc: "Verifies NVMe TBW reporting works as expected",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		HardwareDeps: hwdep.D(hwdep.Nvme()),
		Requirements: []string{
			tdreq.StorageTBWReport,
			tdreq.NvmeStorageHealthInfo,
		},
	})
}

const tbwProbeSize = 512 * 1024 * 1024 // 512 MiB

func NvmeTBW(ctx context.Context, s *testing.State) {
	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	smartInfo, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "smartctl", "-a", disk.Path)
	if err != nil {
		s.Fatal("Could not get SMART for the device: ", err)
	}

	tbw, err := util.GetTBWFromInfo(ctx, smartInfo)
	if err != nil {
		s.Fatal("Could not get TBW from SMART: ", err)
	}

	perfValues := perf.NewValues()

	err = util.TestConfig{}.
		WithPath(disk.Path).
		WithSize(tbwProbeSize).
		WithJobFromFile(s.DataPath("tbw_probe")).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	smartInfo, err = util.RunCmdWithStringOutput(ctx, s.DUT(), "smartctl", "-a", disk.Path)
	if err != nil {
		s.Fatal("Could not get SMART for the device: ", err)
	}

	newTbw, err := util.GetTBWFromInfo(ctx, smartInfo)
	if err != nil {
		s.Fatal("Could not get TBW from SMART, after FIO: ", err)
	}

	perfValues.Set(perf.Metric{
		Name:      "_TBW_Val",
		Unit:      "bytes",
		Direction: perf.BiggerIsBetter,
	}, float64(tbw))

	perfValues.Set(perf.Metric{
		Name:      "_TBW_Diff",
		Unit:      "bytes",
		Direction: perf.BiggerIsBetter,
	}, float64(newTbw-tbw))

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}
}
