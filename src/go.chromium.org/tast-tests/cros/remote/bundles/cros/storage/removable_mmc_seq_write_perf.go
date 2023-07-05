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

func init() {
	testing.AddTest(&testing.Test{
		Func: RemovableMmcSeqWritePerf,
		Desc: "Measure sequential write performance of mmc-card",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Data:         util.Configs,
		Requirements: []string{tdreq.RemovableStorageSeqTp},
		Timeout:      10 * time.Minute,
	})
}

func RemovableMmcSeqWritePerf(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetRemovableSD(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get removable disk: ", err)
	}

	err = util.TestConfig{}.
		WithResultWriter(resultWriter).
		WithDisk(disk).
		WithRunTimeSec(300).
		WithJobFromFile(s.DataPath("seq_write")).
		Run(ctx, s.DUT())

	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}
}
