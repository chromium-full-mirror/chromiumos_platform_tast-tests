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
		Func: IsEmmcCheck,
		Desc: "Checks if the internal storage device is eMMC",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Requirements: []string{tdreq.EmmcInterface},
	})
}

func IsEmmcCheck(ctx context.Context, s *testing.State) {
	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	if disk.Type != util.EmmcDisk {
		s.Fatalf("Wrong storage device type, wanted %s, got %s", util.DiskTypeToString(util.EmmcDisk), util.DiskTypeToString(disk.Type))
	}
}
