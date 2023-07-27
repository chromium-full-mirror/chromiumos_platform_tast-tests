// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HostSpeedtest,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs Speedtest on cellular interface and capture power consumption data",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com", "rmao@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_power", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_perf_nightly", "cellular_crosbolt_unstable"},
		HardwareDeps: hwdep.D(hwdep.Cellular()),
		Timeout:      6 * time.Minute,
		Fixture:      "cellularPower",
	})
}

func HostSpeedtest(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	verifyHostIPSpeedTest := func(ctx context.Context) error {
		const powerBaselineTime = 1 * time.Minute
		//GoBigSleepLint: provide power baseline before speedtest
		testing.Sleep(ctx, powerBaselineTime)

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

		//GoBigSleepLint: provide power baseline after speedtest
		testing.Sleep(ctx, powerBaselineTime)
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPSpeedTest); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
