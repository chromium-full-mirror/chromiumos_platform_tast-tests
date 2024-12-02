// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemPowerOffVerification,
		Desc:         "Verifies that modem can be powered off and then on properly",
		Contacts:     []string{"cros-cellular-core@google.com", "rmao@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "group:cellular_crosbolt", "cellular_crosbolt_perf_nightly"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.CellularModemType(cellularconst.ModemTypeFM101)),
		SoftwareDeps: []string{"modemfwd"},
	})
}

func ModemPowerOffVerification(ctx context.Context, s *testing.State) {
	const MaxCellularDisableRetryNum = 2
	perfValues := perf.NewValues()
	helper := s.FixtValue().(*cellular.FixtData).Helper

	defer func(ctx context.Context) {
		if err := upstart.StopJob(ctx, modemfwd.JobName); err != nil {
			s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
		}
		s.Log("Modemfwd has stopped successfully")
		helper.EnsureEnabled(ctx)
	}(ctx)

	// Modemfwd is initially stopped in the fixture setup. start it for modem power
	// on off
	if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
		s.Fatal("Modemfwd failed during initialization (precondition): ", err)
	}

	if err := helper.EnsureEnabled(ctx); err != nil {
		s.Fatal("Modem EnsureEnabled failed (precondition): ", err)
	}

	// Disable cellular fails in case a modem start is in progress. Retry solves the issue.
	for i := 1; i <= MaxCellularDisableRetryNum; i++ {
		if _, err := helper.Disable(ctx); err != nil && i == MaxCellularDisableRetryNum {
			s.Fatal("Failed to disable modem: ", err)
		}
	}

	// Modem is expected to be turned off when the power off hysteresis
	// timer expires after modem is disabled.
	s.Log("Checking to make sure modem disappears from the bus")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := modemmanager.NewModem(ctx); err == nil {
			return errors.Wrap(err, "modem did not disappear")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 500 * time.Millisecond,
	}); err != nil {
		s.Fatal("Modem power off failed: ", err)
	}

	s.Log("Confirmed modem is powered off. Now enable cellular to turn on the modem")
	// |powerUpTime|: time from power up is triggered to the modem is
	// in enabled state.
	powerUpTime, err := helper.Enable(ctx)
	if err != nil {
		s.Fatal("Failed to enable modem: ", err)
	}
	s.Logf("Modem power up time (till reaching enabled state): %5.2f(s)", powerUpTime.Seconds())

	perfValues.Append(perf.Metric{
		Name:      "modem_power_up_time",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, powerUpTime.Seconds())

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}
