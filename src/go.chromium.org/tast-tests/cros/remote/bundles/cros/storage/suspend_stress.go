// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

// timeParam contains the timing parameters for the test run
type timeParams struct {
	fioTimeSec        int
	suspendIterations int
	timeoutMin        int
}

const (
	fullFioTimeSec         = 19800
	quickFioTimeSec        = 1800
	fullSuspendIterations  = 1000
	quickSuspendIterations = 50
	fullPollTimeoutMin     = 420
	quickPollTimeoutMin    = 90
	fullTimeoutMin         = 450
	quickTimeoutMin        = 120
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
		Vars:         []string{"servo"},
		Params: []testing.Param{
			{
				Val:               timeParams{fioTimeSec: fullFioTimeSec, suspendIterations: fullSuspendIterations, timeoutMin: fullPollTimeoutMin},
				Timeout:           fullTimeoutMin * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageSuspend},
			}, {
				Name:              "iteration_2",
				Val:               timeParams{fioTimeSec: fullFioTimeSec, suspendIterations: fullSuspendIterations, timeoutMin: fullPollTimeoutMin},
				Timeout:           fullTimeoutMin * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageSuspend},
			}, {
				Name:              "iteration_3",
				Val:               timeParams{fioTimeSec: fullFioTimeSec, suspendIterations: fullSuspendIterations, timeoutMin: fullPollTimeoutMin},
				Timeout:           fullTimeoutMin * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageSuspend},
			}, {
				Name:      "quick",
				Val:       timeParams{fioTimeSec: quickFioTimeSec, suspendIterations: quickSuspendIterations, timeoutMin: quickPollTimeoutMin},
				Timeout:   quickTimeoutMin * time.Minute,
				ExtraAttr: []string{"group:storage-qual", "storage-qual_avl_v3"},
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

	pxy, err := servo.NewProxy(ctx, s.RequiredVar("servo"), s.DUT().KeyFile(), s.DUT().KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	svo := pxy.Servo()
	defer pxy.Close(ctx)

	err = util.WriteAVLInfo(ctx, disk, s.OutDir())
	if err != nil {
		s.Fatal("Failed to write AVL info: ", err)
	}

	params := s.Param().(timeParams)

	t := util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk).
		WithRunTimeSec(params.fioTimeSec).
		WithJobFromFile(s.DataPath("suspend_stress"))
	pidFio, err := t.RunBackground(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	pidSuspend, err := util.SuspendStressTest(ctx, s.DUT(), params.suspendIterations)
	if err != nil {
		s.Fatal("Failed to run suspend stress test: ", err)
	}

	if err := s.DUT().Disconnect(ctx); err != nil {
		s.Fatal("Failed to close the current DUT ssh connection: ", err)
	}

	// GoBigSleepLint: Wait for workloads to finish before reestablishing connection
	// to avoid flakiness from connection loss during suspend/resume.
	if err := testing.Sleep(ctx, time.Duration(params.timeoutMin)*time.Minute); err != nil {
		s.Fatal("Sleep failed: ", err)
	}

	ok, err := svo.HasControl(ctx, string(servo.DutEthPwrEn))
	if err != nil {
		s.Fatalf("Failed to check control %v: %v", servo.DutEthPwrEn, err)
	}
	if !ok {
		s.Fatal("Servo does not have control ", servo.DutEthPwrEn)
	}
	testing.ContextLog(ctx, "Resetting the ethernet adapter")
	if err := svo.ToggleOffOn(ctx, servo.DutEthPwrEn); err != nil {
		s.Fatal("Failed to toggle servo control ", servo.DutEthPwrEn)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return s.DUT().Connect(ctx)
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	cmd := "lsof -p " + pidFio + " -p " + pidSuspend
	out, _ := util.RunCmdWithStringOutputSilent(ctx, s.DUT(), "bash", "-c", cmd)

	if out != "" {
		s.Log(out)
		s.Fatal("Tasks did not complete in expected time")
	}

	util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	if err = util.CheckSuspendStressResults(ctx, s.DUT()); err != nil {
		s.Fatal("Suspend failure: ", err)
	}

	if err = t.ParseFioResults(ctx, s.DUT()); err != nil {
		s.Fatal("Fio failure: ", err)
	}
}
