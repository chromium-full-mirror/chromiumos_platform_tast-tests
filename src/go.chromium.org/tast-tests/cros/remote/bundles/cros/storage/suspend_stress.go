// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendStress,
		Desc: "Test suspend/resume functionality in parallel with disk activity",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Timeout:      450 * time.Minute,
		Requirements: []string{
			tdreq.StorageSuspend,
		},
		Params: []testing.Param{
			{
				ExtraAttr:    []string{"group:storage-qual", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
			}, {
				Name: "iteration_2",
				ExtraAttr:    []string{"group:storage-qual", "storage-qual_avl_v3"},
			}, {
				Name: "iteration_3",
				ExtraAttr:    []string{"group:storage-qual", "storage-qual_avl_v3"},
			},
		},
	})
}

func SuspendStress(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)
	defer util.CleanupFio(ctx, s.DUT())
	defer util.CleanupSuspend(ctx, s.DUT())

	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	t := util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk).
		WithRunTimeSec(19800).
		WithJobFromFile(s.DataPath("suspend_stress"))
	pidFio, err := t.RunBackground(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	pidSuspend, err := util.SuspendStressTest(ctx, s.DUT(), 1000)
	if err != nil {
		s.Fatal("Failed to run suspend stress test: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		util.FatalIfBootIDChanged(ctx, bootIDChecker, s)
		cmd := "lsof -p " + pidFio + " -p " + pidSuspend + " +r 1 &>/dev/null"
		out, err := util.RunCmdWithStringOutputSilent(ctx, s.DUT(), "bash", "-c", cmd)
		if err != nil {
			return errors.Wrap(err, "failed to listen for pid done")
		}
		s.Log(out)
		return nil
	}, &testing.PollOptions{Timeout: 7 * time.Hour}); err != nil {
		s.Fatal("Timed out listening for pid done: ", err)
	}

	if err = util.CheckSuspendStressResults(ctx, s.DUT()); err != nil {
		s.Fatal("Suspend failure: ", err)
	}

	if err = t.ParseFioResults(ctx, s.DUT()); err != nil {
		s.Fatal("Fio failure: ", err)
	}
}
