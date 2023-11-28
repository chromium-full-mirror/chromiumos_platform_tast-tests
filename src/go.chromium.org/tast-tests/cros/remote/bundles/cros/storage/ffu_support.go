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
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FfuSupport,
		Desc: "Check FFU is reported as supported by storage",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Attr:    []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Requirements: []string{
			tdreq.StorageFFU,
		},
	})
}

func FfuSupport(ctx context.Context, s *testing.State) {
	const storageInfoPath = "/mnt/stateful_partition/encrypted/var/log/storage_info.txt"

	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	ffu := float64(0)
	perfValues := perf.NewValues()

	switch disk.Type {
	case util.NvmeDisk:
		identity, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "nvme", "id-ctrl", disk.Path)
		if err != nil {
			s.Fatal("Could not read controller identity: ", err)
		}
		if oacs, ok := util.FindSingleHexInt64NvmeRegs(ctx, identity, "oacs"); ok {
			ffu = float64(oacs & 4) // Bit 2 of oacs
		}
	case util.EmmcOverNvmeDisk:
		// BH799 supports FFU
		ffu = float64(1)
	case util.EmmcDisk:
		ffuInt, err := disk.ReadSysfsInt64(ctx, "device/ffu_capable")
		if err != nil {
			s.Fatal("Failed to read FFU capability: ", err)
		}
		ffu = float64(ffuInt)
	case util.UfsDisk:
		storageInfo, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "cat", storageInfoPath)
		if err != nil {
			s.Fatal("Could not read storage info from the device: ", err)
		}

		if feats, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, "bUFSFeaturesSupport"); ok {
			ffu = float64(feats & 1) // bit 0 of the feature field
		}
	default:
		ffu = float64(0)
	}

	perfValues.Set(perf.Metric{
		Name:      "_FFU",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, ffu)

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}
}
