// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type persistEnabledTestParams struct {
	enabledState bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillSuspendResumePersistEnabled,
		Desc:         "Verifies that cellular maintains enabled state around Suspend/Resume",
		Contacts:     []string{"chromeos-cellular-team@google.com", "danielwinkler@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active"},
		Fixture:      "cellularSuspendLocal",
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

func ShillSuspendResumePersistEnabled(ctx context.Context, s *testing.State) {
	params := s.Param().(persistEnabledTestParams)

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Apply required enabled state
	if params.enabledState {
		if _, err := helper.Enable(ctx); err != nil {
			s.Fatal("Unable to enable Cellular: ", err)
		}
	} else {
		if _, err := helper.Disable(ctx); err != nil {
			s.Fatal("Unable to disable Cellular: ", err)
		}
	}

	// Request suspend for 10 seconds
	if _, err := suspend.ForDuration(ctx, 10*time.Second); err != nil {
		s.Fatal("The powerd_dbus_suspend failed to properly suspend: ", err)
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
