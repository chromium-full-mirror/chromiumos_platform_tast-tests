// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strconv"
	"strings"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HealthInfo,
		Desc: "Check health info is reported by storage",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:    []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		Data:         util.Configs,
		Requirements: []string{
			tdreq.StorageHealthReport,
		},
	})
}

func emmcHealthSupported(ctx context.Context, disk *util.Disk) (bool, error) {
	lifeStr, err := disk.ReadSysfsString(ctx, "device/life_time")
	if err != nil {
		return false, errors.Wrap(err, "failed to read eMMC lifetime")
	}

	lifes := strings.Split(lifeStr, " ")
	lifeA, errA := strconv.ParseInt(lifes[0][2:], 16, 64)
	lifeB, errB := strconv.ParseInt(lifes[1][2:], 16, 64)
	if errA != nil {
		return false, errA
	}
	if errB != nil {
		return false, errB
	}
	if lifeA == 0 && lifeB == 0 {
		return false, nil
	}

	eol, err := disk.ReadSysfsHexInt64(ctx, "device/pre_eol_info")
	if err != nil {
		return false, errors.Wrap(err, "failed to read eMMC eol info")
	}

	return eol > 0, nil
}

func ufsHealthSupported(ctx context.Context, dut *dut.DUT) (bool, error) {
	const storageInfoPath = "/mnt/stateful_partition/encrypted/var/log/storage_info.txt"
	storageInfo, err := util.RunCmdWithStringOutput(ctx, dut, "cat", storageInfoPath)
	if err != nil {
		return false, errors.Wrap(err, "could not read storage info from the device")
	}
	lifeA, okA := util.FindSingleInt64ValueUfs(ctx, storageInfo, "bDeviceLifeTimeEstA")
	lifeB, okB := util.FindSingleInt64ValueUfs(ctx, storageInfo, "bDeviceLifeTimeEstB")
	eol, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, "bPreEOLInfo")

	if !okA || !okB || !ok {
		return false, errors.New("can't read health descriptor of a ufs device")
	}
	if lifeA == 0 && lifeB == 0 {
		return false, nil
	}
	return eol > 0, nil
}

func nvmeHealthSupported(ctx context.Context, dut *dut.DUT, probeFIO string) (bool, error) {
	disk, err := util.GetStandbyRootfs(ctx, dut)
	if err != nil {
		return false, errors.Wrap(err, "failed to get internal disk")
	}

	smartInfo, err := util.RunCmdWithStringOutput(ctx, dut, "smartctl", "-a", disk.Path)
	if err != nil {
		return false, errors.Wrap(err, "could not get SMART for the device")
	}

	tbw, err := util.GetTBWFromInfo(ctx, smartInfo)
	if err != nil {
		return false, errors.Wrap(err, "could not get TBW from SMART")
	}

	const tbwProbeSize = 512 * 1024 * 1024 // 512 MiB
	err = util.TestConfig{}.
		WithPath(disk.Path).
		WithSize(tbwProbeSize).
		WithJobFromFile(probeFIO).
		Run(ctx, dut)

	if err != nil {
		return false, errors.Wrap(err, "failed to run FIO")
	}

	smartInfo, err = util.RunCmdWithStringOutput(ctx, dut, "smartctl", "-a", disk.Path)
	if err != nil {
		return false, errors.Wrap(err, "could not get SMART for the device")
	}

	newTbw, err := util.GetTBWFromInfo(ctx, smartInfo)
	if err != nil {
		return false, errors.Wrap(err, "could not get TBW from SMART, after FIO")
	}

	tbwDelta := float64(newTbw - tbw)
	tbwDeltaMin := float64(tbwProbeSize) * 0.9
	tbwDeltaMax := float64(tbwProbeSize) * 1.1

	if tbw < (1 << 30) {
		return false, errors.New("suspiciously low TBW")
	}

	if tbwDelta < tbwDeltaMin && tbwDelta > tbwDeltaMax {
		return false, errors.Errorf("unexpected delta TBW: %f, want within (%f, %f)", tbwDelta, tbwDeltaMin, tbwDeltaMax)
	}

	return true, nil
}

func HealthInfo(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	healthSupported := false

	switch disk.Type {
	case util.NvmeDisk:
		fallthrough
	case util.EmmcOverNvmeDisk:
		healthSupported, err = nvmeHealthSupported(ctx, s.DUT(), s.DataPath("tbw_probe"))
	case util.EmmcDisk:
		healthSupported, err = emmcHealthSupported(ctx, disk)
	case util.UfsDisk:
		healthSupported, err = ufsHealthSupported(ctx, s.DUT())
	default:
		s.Fatal("Unsupported device type")
	}

	if err != nil {
		s.Fatal("Error determining health status support: ", err)
	}

	if !healthSupported {
		s.Fatal("Health reporting is not supported")
	}
}
