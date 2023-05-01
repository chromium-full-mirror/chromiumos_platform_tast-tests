// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	tdreq "chromiumos/tast/common/testdevicerequirements"
	"chromiumos/tast/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RemovableMmcSeqReadPerf,
		Desc: "Measure sequential read performance of mmc-card",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567",
		Data:         util.Configs,
		Requirements: []string{tdreq.RemovableStorageSeqTp},
	})
}

func RemovableMmcSeqReadPerf(ctx context.Context, s *testing.State) {
	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetRemovableMmc(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get removable disk: ", err)
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
