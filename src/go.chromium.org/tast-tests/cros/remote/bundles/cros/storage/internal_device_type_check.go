// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage contains tests for storage.
package storage

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/bounds"
	"go.chromium.org/tast-tests/cros/common/perf"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: InternalDeviceTypeCheck,
		Desc: "Checks if the internal storage is of the right type and size",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		Requirements: []string{
			tdreq.InternalStorageInterface,
			tdreq.StorageEmmcCapacity,
			tdreq.StorageCapacityMin,
		},
	})
}

func InternalDeviceTypeCheck(ctx context.Context, s *testing.State) {
	metricBounds := []bounds.MetricBounds{{
		Metric: bounds.MatchExact("_DiskSize"),
		Bounds: bounds.Min(30_400_000_000), // 32 GB (power of 10) minus 5%
	}, {
		Metric: bounds.MatchExact("_EmmcDiskSize"),
		Bounds: bounds.Max(128_000_000_000), // 128 GB (power of 10)
	}}
	defer util.FatalIfBoundsCheckFail(ctx, metricBounds, s)

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

	ifaces := map[string]bool{
		util.DiskTypeToString(util.UfsDisk):          true,
		util.DiskTypeToString(util.EmmcDisk):         true,
		util.DiskTypeToString(util.NvmeDisk):         true,
		util.DiskTypeToString(util.EmmcOverNvmeDisk): true,
	}

	perfValues := perf.NewValues()

	perfValues.Set(perf.Metric{
		Name:      "_DiskSize",
		Unit:      "bytes",
		Direction: perf.BiggerIsBetter,
	}, float64(disk.Size))

	// We are to impose additional constraints on the eMMC devices,
	// So record an additional metric to make PVS check upon.
	// TODO(dlunev): Add higher capacity check.
	emmcSize := 0
	if disk.Type == util.EmmcDisk || disk.Type == util.EmmcOverNvmeDisk {
		emmcSize = disk.Size
	}

	perfValues.Set(perf.Metric{
		Name:      "_EmmcDiskSize",
		Unit:      "bytes",
		Direction: perf.BiggerIsBetter,
	}, float64(emmcSize))

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}

	if _, ok := ifaces[util.DiskTypeToString(disk.Type)]; !ok {
		s.Fatalf("Unexepcted storage interface, want one of %v, got %s", ifaces, util.DiskTypeToString(disk.Type))
	}
}
