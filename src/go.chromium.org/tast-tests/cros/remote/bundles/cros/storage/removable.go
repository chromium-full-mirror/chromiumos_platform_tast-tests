// Copyright 2024 The ChromiumOS Authors
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
		Func: Removable,
		Desc: "Verify basic functionality of the SD card reader",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Timeout:      90 * time.Minute,
		Requirements: []string{
			tdreq.CardReader,
		},
	})
}

func Removable(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	resultWriter := &util.FioResultWriter{}
	defer resultWriter.Save(ctx, s.OutDir(), true)

	disk, err := util.GetRemovableSD(ctx, s.DUT())
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

	for _, job := range []string{"seq_write", "seq_read"} {
		err = configBase.
			WithRunTimeSec(30).
			WithJobFromFile(s.DataPath(job)).
			Run(ctx, s.DUT())
		if err != nil {
			s.Fatal("Failed to run fio: ", err)
		}
	}

	err = configBase.
		WithJobFromFile(s.DataPath("8k_async_randwrite")).
		Run(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	pidSuspend, err := util.SuspendStressTest(ctx, s.DUT(), 10)
	if err != nil {
		s.Fatal("Failed to run suspend stress test: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := "lsof -p " + pidSuspend + " +r 1 &>/dev/null"
		out, err := util.RunCmdWithStringOutputSilent(ctx, s.DUT(), "bash", "-c", cmd)
		if err != nil {
			return errors.Wrap(err, "failed to listen for pid done")
		}
		s.Log(out)
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Minute}); err != nil {
		s.Fatal("Timed out listening for pid done: ", err)
	}

	util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	err = configBase.
		WithJobFromFile(s.DataPath("8k_async_randwrite")).
		WithVerifyOnly(true).
		Run(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to run fio: ", err)
	}

	if err = util.CheckSuspendStressResults(ctx, s.DUT()); err != nil {
		s.Fatal("Suspend failure: ", err)
	}
}
