// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectivityPerf,
		Desc:         "Measure the time it takes to enable, disable, connect, and disconnect from a Cellular Service ",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular_crosbolt", "cellular_crosbolt_perf_nightly", "cellular_crosbolt_unstable"},
		HardwareDeps: hwdep.D(hwdep.Cellular()),
		Timeout:      10 * time.Minute,
		Fixture:      "cellular",
		Requirements: []string{"cell-hwSys-0009-v01", "cell-hwSys-0010-v01", "cell-hwSys-0011-v01", "cell-hwSys-0012-v01"},
	})
}

func ConnectivityPerf(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	verifyCellularConnectivity := func(ctx context.Context) error {
		perfValues := perf.NewValues()

		// Test Disable / Enable / Connect / Disconnect.
		// Run the test a second time to test Disable after Connect/Disconnect.
		// Run the test a third time to help test against flakiness.
		for i := 0; i < 3; i++ {
			testing.ContextLogf(ctx, "Disable %d", i)
			disableTime, err := helper.Disable(ctx)
			if err != nil {
				return errors.Wrapf(err, "disable failed on attempt %d", i)
			}
			perfValues.Append(perf.Metric{
				Name:      "cellular_disable_time",
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, disableTime.Seconds())
			testing.ContextLogf(ctx, "Enable %d", i)

			// GoBigSleepLint: sleep to prevent MM from throttling.
			testing.Sleep(ctx, 2*time.Second)

			enableTime, err := helper.Enable(ctx)
			if err != nil {
				return errors.Wrapf(err, "enable failed on attempt %d", i)
			}
			perfValues.Append(perf.Metric{
				Name:      "cellular_enable_time",
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, enableTime.Seconds())
			testing.ContextLogf(ctx, "Connect %d", i)
			connectTime, err := helper.ConnectToDefault(ctx)
			if err != nil {
				return errors.Wrapf(err, "connect failed on attempt %d", i)
			}
			perfValues.Append(perf.Metric{
				Name:      "cellular_connect_time",
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, connectTime.Seconds())
			testing.ContextLogf(ctx, "Disconnect %d", i)
			disconnectTime, err := helper.Disconnect(ctx)
			if err != nil {
				return errors.Wrapf(err, "disconnect failed on attempt %d", i)
			}
			perfValues.Append(perf.Metric{
				Name:      "cellular_disconnect_time",
				Unit:      "seconds",
				Direction: perf.SmallerIsBetter,
				Multiple:  true,
			}, disconnectTime.Seconds())
		}

		if err := perfValues.Save(s.OutDir()); err != nil {
			return errors.Wrap(err, "failed saving perf data")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyCellularConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}

}
