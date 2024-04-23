// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

const (
	fullBenchmarkTime  = 300
	quickBenchmarkTime = 30
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StressWrite,
		Desc: "Compares performance of the storage device after a stressful workload",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Params: []testing.Param{
			{
				Val:               fullBenchmarkTime,
				Timeout:           fullBenchmarkTime * 1.5 * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageStable, tdreq.StorageEndurancePerf},
			}, {
				Name:              "iteration_2",
				Val:               fullBenchmarkTime,
				Timeout:           fullBenchmarkTime * 1.5 * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageStable, tdreq.StorageEndurancePerf},
			}, {
				Name:              "iteration_3",
				Val:               fullBenchmarkTime,
				Timeout:           fullBenchmarkTime * 1.5 * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageStable, tdreq.StorageEndurancePerf},
			}, {
				Name:    "quick",
				Val:     quickBenchmarkTime,
				Timeout: quickBenchmarkTime * 1.5 * time.Minute,
			},
		},
	})
}

func StressWrite(ctx context.Context, s *testing.State) {
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

	configBase := util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk)

	benchTime := s.Param().(int)
	stressTime := benchTime * 60

	// Get initial performance results
	// GoBigSleepLint: Provide some idle time before measuring storage performance.
	if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	for _, job := range []string{"seq_write", "seq_read", "4k_write", "4k_read"} {
		err = configBase.
			WithRunTimeSec(benchTime).
			WithJobFromFile(s.DataPath(job)).
			Run(ctx, s.DUT())
		if err != nil {
			s.Fatal("Failed to run fio: ", err)
		}
	}

	util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	// Stress the device, verifying data along the way
	duration := stressTime / 60
	s.Logf("Starting 64k_stress workload, will run for %d minutes", duration)
	err = configBase.
		WithRunTimeSec(stressTime).
		WithJobFromFile(s.DataPath("64k_stress")).
		Run(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	// Get after performance results
	// GoBigSleepLint: Provide some idle time before measuring storage performance.
	if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	for _, job := range []string{"seq_write", "seq_read", "4k_write", "4k_read"} {
		err = configBase.
			WithRunTimeSec(benchTime).
			WithJobFromFile(s.DataPath(job)).
			Run(ctx, s.DUT())
		if err != nil {
			s.Fatal("Failed to run fio: ", err)
		}
	}

}
