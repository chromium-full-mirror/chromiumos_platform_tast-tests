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
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SocPerformance,
		Desc: "Measure performance of SoC storage link",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.USBDevModeWithReinstall,
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			Name:              "nvme_link_bw",
			ExtraRequirements: []string{tdreq.NvmePcieBW},
		}, {
			Name:              "emmc_link_bw",
			ExtraRequirements: []string{tdreq.EmmcControllerBW},
		}, {
			Name:              "ufs_g3_link_bw",
			ExtraRequirements: []string{tdreq.UfsControllerG3BW},
		}, {
			Name:              "ufs_g4_link_bw",
			ExtraRequirements: []string{tdreq.UfsControllerG4BW},
		}},
	})
}

func SocPerformance(ctx context.Context, s *testing.State) {
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
		WithJobFromFile(s.DataPath("seq_read")).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}
}
