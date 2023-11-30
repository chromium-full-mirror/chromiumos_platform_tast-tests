// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NvmeConfiguration,
		Desc: "Verifies NVMe parameters are as expected",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:    []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		HardwareDeps: hwdep.D(hwdep.Nvme()),
		Requirements: []string{
			tdreq.NvmeStorageOnPcie,
			tdreq.NvmeStoragePcieGen,
			tdreq.NvmeStorageProtocolVersion,
		},
	})
}

func NvmeConfiguration(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	identity, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "nvme", "id-ctrl", disk.Path)
	if err != nil {
		s.Fatal("Could not read controller identity: ", err)
	}

	perfValues := perf.NewValues()

	version, ok := util.FindSingleHexInt64NvmeRegs(ctx, identity, "ver")
	if !ok {
		s.Fatal("Could not verify NVMe version")
	}

	perfValues.Set(perf.Metric{
		Name:      "_NvmeVersion",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, float64(version))

	isPcieTransport := float64(0)
	transport, err := disk.ReadSysfsString(ctx, "device/transport")
	if err != nil {
		s.Fatal("Can't determine NVMe transport: ", err)
	}

	if transport == "pcie" {
		isPcieTransport = float64(1)
	}

	perfValues.Set(perf.Metric{
		Name:      "_NvmePCIe",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, isPcieTransport)

	linkSpeedStr, err := disk.ReadSysfsString(ctx, "device/device/current_link_speed")
	if err != nil {
		s.Fatal("Can't determine NVMe link speed: ", err)
	}

	linkSpeedStrVal := strings.Split(linkSpeedStr, " ")[0]
	linkSpeed, err := strconv.ParseFloat(linkSpeedStrVal, 64)
	if err != nil {
		s.Fatalf("Failed to parse %q as float64 from %q: %v", linkSpeedStrVal, linkSpeedStr, err)
	}

	perfValues.Set(perf.Metric{
		Name:      "_NvmeLinkSpeed",
		Unit:      "GT_s",
		Direction: perf.BiggerIsBetter,
	}, float64(linkSpeed))

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}
}
