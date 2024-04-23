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

const hs400esBit = 20
const cqeBit = 23

func init() {
	testing.AddTest(&testing.Test{
		Func: EmmcConfiguration,
		Desc: "Check eMMC parameters are as expected",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		HardwareDeps: hwdep.D(hwdep.Emmc()),
		Requirements: []string{
			tdreq.EmmcStorageControllerRevision,
			tdreq.EmmcStorageControllerHS,
			tdreq.EmmcStorageControllerCqeSupport,
			tdreq.EmmcStorageControllerCqeActive,
			tdreq.EmmcStorageDeviceRevision,
			tdreq.EmmcStorageDeviceHS,
		},
	})
}

func EmmcConfiguration(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	err = util.WriteAVLInfo(ctx, disk, s.OutDir())
	if err != nil {
		s.Fatal("Failed to write AVL info: ", err)
	}

	perfValues := perf.NewValues()

	emmcRevision, err := disk.ReadSysfsHexInt64(ctx, "device/rev")
	if err != nil {
		s.Fatal("Failed to read eMMC revision: ", err)
	}

	perfValues.Set(perf.Metric{
		Name:      "_EMMC_Revision",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, float64(emmcRevision))

	caps2, err := disk.ReadDebugfsHexInt64(ctx, "caps2")
	if err != nil {
		s.Fatal("Failed to read eMMC capabilities: ", err)
	}

	hs400es := float64(0)
	cqe := float64(0)

	if caps2&(1<<hs400esBit) > 0 {
		hs400es = float64(1)
	}

	if caps2&(1<<cqeBit) > 0 {
		cqe = float64(1)
	}

	perfValues.Set(perf.Metric{
		Name:      "_EMMC_HS400ES",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, hs400es)

	perfValues.Set(perf.Metric{
		Name:      "_EMMC_CQE",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, cqe)

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}
}
