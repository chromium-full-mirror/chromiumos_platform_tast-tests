// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IsUfsCheck,
		Desc: "Checks if the internal storage device is UFS",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:    []string{"group:storage-qual", "storage-qual_pdp_enabled", "storage-qual_pdp_kpi", "storage-qual_pdp_stress", "storage-qual_avl_v3"},
		Requirements: []string{tdreq.UfsInterface},
	})
}

func IsUfsCheck(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	if disk.Type != util.UfsDisk {
		s.Fatalf("Wrong storage device type, wanted %s, got %s", util.DiskTypeToString(util.UfsDisk), util.DiskTypeToString(disk.Type))
	}
}
