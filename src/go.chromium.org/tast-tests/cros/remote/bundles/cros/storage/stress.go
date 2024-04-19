// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"strings"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

const (
	fullStressTimeSec     = "21600"
	quickStressTimeSec    = "3600"
	fullStressTimeoutMin  = 420
	quickStressTimeoutMin = 90
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Stress,
		Desc: "Run stressapptest",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Params: []testing.Param{
			{
				Val:               fullStressTimeSec,
				Timeout:           fullStressTimeoutMin * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageStable},
			}, {
				Name:              "iteration_2",
				Val:               fullStressTimeSec,
				Timeout:           fullStressTimeoutMin * time.Minute,
				ExtraAttr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
				ExtraRequirements: []string{tdreq.StorageStable},
			}, {
				Name:    "quick",
				Val:     quickStressTimeSec,
				Timeout: quickStressTimeoutMin * time.Minute,
			},
		},
	})
}

func Stress(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	runTimeSec := s.Param().(string)
	out, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "stressapptest", "-m", "8", "-s", runTimeSec, "-d", disk.Path)
	if err != nil {
		s.Fatal("Failed to run stressapptest: ", err)
	}

	if !strings.Contains(out, "PASS") {
		s.Fatalf("stressapptest failed: %s", out)
	}
}
