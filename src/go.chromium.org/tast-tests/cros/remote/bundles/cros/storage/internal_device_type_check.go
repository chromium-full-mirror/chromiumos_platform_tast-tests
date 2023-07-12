// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage contains tests for storage.
package storage

import (
	"context"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: InternalDeviceTypeCheck,
		Desc: "Checks if the internal storage is of the right type",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Requirements: []string{tdreq.InternalStorageInterface},
	})
}

func InternalDeviceTypeCheck(ctx context.Context, s *testing.State) {
	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	ifaces := map[string]bool{
		util.DiskTypeToString(util.UfsDisk):          true,
		util.DiskTypeToString(util.EmmcDisk):         true,
		util.DiskTypeToString(util.NvmeDisk):         true,
		util.DiskTypeToString(util.EmmcOverNvmeDisk): true,
	}

	if _, ok := ifaces[util.DiskTypeToString(disk.Type)]; !ok {
		s.Fatalf("Unexepcted storage interface, want one of %v, got %s", ifaces, util.DiskTypeToString(disk.Type))
	}
}
