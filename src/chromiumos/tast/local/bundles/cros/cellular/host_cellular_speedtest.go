// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HostCellularSpeedtest,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs Speedtest on cellular interface",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular_crosbolt", "cellular_crosbolt_perf_nightly", "cellular_crosbolt_unstable"},
		HardwareDeps: hwdep.D(hwdep.Cellular()),
		Timeout:      4 * time.Minute,
	})
}

func HostCellularSpeedtest(ctx context.Context, s *testing.State) {
	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create connected cellular.Helper (precondition): ", err)
	}

	verifyHostIPSpeedTest := func(ctx context.Context) error {
		uploadSpeed, downloadSpeed, err := cellular.RunHostIPSpeedTest(ctx, testexec.CommandContext, "/usr/local/bin")
		if err != nil {
			return errors.Wrap(err, "failed speed test")
		}
		if uploadSpeed == 0.0 || downloadSpeed == 0.0 {
			return errors.Errorf("invalid speed upload: %f download: %f", uploadSpeed, downloadSpeed)
		}
		perfValues := perf.NewValues()
		perfValues.Set(perf.Metric{
			Name:      "Upload",
			Unit:      "bps",
			Direction: perf.BiggerIsBetter,
		}, uploadSpeed)
		perfValues.Set(perf.Metric{
			Name:      "Download",
			Unit:      "bps",
			Direction: perf.BiggerIsBetter,
		}, downloadSpeed)
		if err := perfValues.Save(s.OutDir()); err != nil {
			return errors.Wrap(err, "failed saving perf data")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPSpeedTest); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
