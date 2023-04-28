// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/cellular"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type persistEnabledTestParams struct {
	enabledState bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillCellularSuspendResumePersistEnabled,
		Desc:         "Verifies that cellular maintains enabled state around Suspend/Resume",
		Contacts:     []string{"chromeos-cellular-team@google.com", "danielwinkler@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
		// TODO(b/217106877): Skip on herobrine as S/R is unstable
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform("herobrine")),
		Params: []testing.Param{{
			Name: "enabled",
			Val: persistEnabledTestParams{
				enabledState: true,
			},
		}, {
			Name: "disabled",
			Val: persistEnabledTestParams{
				enabledState: false,
			},
		}},
	})
}

func ShillCellularSuspendResumePersistEnabled(ctx context.Context, s *testing.State) {
	params := s.Param().(persistEnabledTestParams)

	helper, _, err := cellular.NewHelperWithSim(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper (precondition): ", err)
	}

	// Apply required enabled state
	if params.enabledState {
		_, err = helper.Enable(ctx)
	} else {
		_, err = helper.Disable(ctx)
	}

	if err != nil {
		s.Fatal("Failed to set initial enabled setting to ", params.enabledState, ": ", err)
	}

	// Request suspend for 10 seconds
	if err := testexec.CommandContext(ctx, "powerd_dbus_suspend", "--suspend_for_sec=10").Run(); err != nil {
		s.Fatal("Failed to perform system suspend (precondition): ", err)
	}

	// Verify enabled setting persisted
	if err := helper.Device.WaitForProperty(ctx, shillconst.DevicePropertyPowered, params.enabledState, shillconst.DefaultTimeout); err != nil {
		s.Fatal("Failed to set enabled to ", params.enabledState, ": ", err)
	}

	// Return to enabled state and confirm service available
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to re-enable modem after resume: ", err)
	}

	if _, err := helper.FindServiceForDevice(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service after resume: ", err)
	}
}
