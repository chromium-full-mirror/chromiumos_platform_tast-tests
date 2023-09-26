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
		Func: StressWrite,
		Desc: "Compares performance of the storage device after a stressful workload",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.USBDevModeWithReinstall,
		Timeout:      450 * time.Minute,
		Requirements: []string{
			tdreq.StorageStable, tdreq.StorageEndurancePerf,
		},
	})
}

func StressWrite(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	configBase := util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk)

	// Get initial performance results
	// GoBigSleepLint: Provide some idle time before measuring storage performance.
	if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	for _, job := range []string{"seq_write", "seq_read", "4k_write", "4k_read"} {
		err = configBase.
			WithRunTimeSec(300).
			WithJobFromFile(s.DataPath(job)).
			Run(ctx, s.DUT())
		if err != nil {
			s.Fatal("Failed to run fio: ", err)
		}
	}

	// Stress the device, verifying data along the way
	s.Log("Starting 64k_stress workload, will run for 5 hours")
	err = configBase.
		WithRunTimeSec(18000).
		WithJobFromFile(s.DataPath("64k_stress")).
		Run(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	// Get after performance results
	// GoBigSleepLint: Provide some idle time before measuring storage performance.
	if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	for _, job := range []string{"seq_write", "seq_read", "4k_write", "4k_read"} {
		err = configBase.
			WithRunTimeSec(300).
			WithJobFromFile(s.DataPath(job)).
			Run(ctx, s.DUT())
		if err != nil {
			s.Fatal("Failed to run fio: ", err)
		}
	}

}
