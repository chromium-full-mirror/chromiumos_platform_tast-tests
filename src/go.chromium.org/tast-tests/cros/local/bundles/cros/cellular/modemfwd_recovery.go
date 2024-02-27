// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/cellularconst"
	"go.chromium.org/tast/core/testing/hwdep"
)

type recoveryTestParams struct {
	connectivityCheck bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModemfwdRecovery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that modemfwd's recovery mechanism works properly",
		Contacts:     []string{"chromeos-cellular-team@google.com", "danielwinkler@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_perf_nightly"},
		Fixture:      "cellular",
		Timeout:      10 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.SkipOnCellularModemType(cellularconst.ModemTypeSC7280)),
		SoftwareDeps: []string{"modemfwd"},
		Params: []testing.Param{{
			Name: "",
			Val: recoveryTestParams{
				connectivityCheck: false,
			},
		}, {
			Name: "connectivity_check",
			Val: recoveryTestParams{
				connectivityCheck: true,
			},
		}},
	})
}

func waitForFileState(ctx context.Context, path string, desiredExistenceState bool, timeout time.Duration) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var fileExists bool = false
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			fileExists = true
		}
		if desiredExistenceState == fileExists {
			return nil
		}
		return errors.New("File not in expected state")
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return err
	}
	return nil
}

// ModemfwdRecovery Test
func ModemfwdRecovery(ctx context.Context, s *testing.State) {
	const MaxRecoveryTime = 3 * time.Minute
	params := s.Param().(recoveryTestParams)
	perfValues := perf.NewValues()

	defer func(ctx context.Context) {
		if err := upstart.StopJob(ctx, modemfwd.JobName); err != nil {
			s.Fatalf("Failed to stop %q: %s", modemfwd.JobName, err)
		}
		s.Log("modemfwd has stopped successfully")
	}(ctx)

	// modemfwd is initially stopped in the fixture SetUp
	if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
		s.Fatal("modemfwd failed during initialization (precondition): ", err)
	}

	// Grab modem's primaryport to keep track of the modem's recovery progress.
	// We expect that the modem endpoints will be refreshed by the kernel
	// driver around recovery. We also expect the primaryport to be the same
	// across recovery, so we only need to fetch it once when the test starts.
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object (precondition): ", err)
	}
	props, err := modem.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to call GetProperties on Modem (precondition): ", err)
	}
	primaryPort, err := props.GetString("PrimaryPort")
	if err != nil {
		s.Fatal("Missing PrimaryPort property (precondition): ", err)
	}
	devPort := "/dev/" + primaryPort

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Ensure initial connectivity
	if params.connectivityCheck {
		if _, err := helper.Connect(ctx); err != nil {
			s.Fatal("Failed to connect to cellular service (precondition): ", err)
		}
	}

	start := time.Now()

	// Emulate broken communications to modem by freezing mbim-proxy. Wait
	// until primary port goes away, indicating the kernel driver has torn
	// down its state as part of recovery procedure.
	if err := testexec.CommandContext(ctx, "killall", "-STOP", "mbim-proxy").Run(); err != nil {
		s.Fatal("Failed to halt mbim-proxy: ", err)
	}
	if err := waitForFileState(ctx, devPort, false, MaxRecoveryTime); err != nil {
		// Clean up our broken mbim-proxy before we abort the test
		if err := testexec.CommandContext(ctx, "killall", "-CONT", "mbim-proxy").Run(); err != nil {
			s.Log("Failed to bring back mbim-proxy: ", err)
		}
		s.Fatal("Modem didn't go away as expected")
	}

	// While modem is recovering, resume mbim-proxy to allow seamless
	// communication once the modem comes back. Wait for primary port to
	// come back, indicating the kernel driver has re-initialized.
	if err := testexec.CommandContext(ctx, "killall", "-CONT", "mbim-proxy").Run(); err != nil {
		s.Fatal("Failed to bring back mbim-proxy: ", err)
	}
	if err := waitForFileState(ctx, devPort, true, MaxRecoveryTime); err != nil {
		s.Fatal("Modem didn't come back as expected")
	}

	if err := helper.EnsureEnabled(ctx); err != nil {
		s.Fatal("Failed to find default Cellular Service: ", err)
	}

	perfValues.Append(perf.Metric{
		Name:      "cellular_recovery_time",
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, time.Since(start).Seconds())

	if params.connectivityCheck {
		// Ensure connectivity after recovery
		if _, err := helper.Connect(ctx); err != nil {
			s.Fatal("Failed to connect to cellular service: ", err)
		}
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}
