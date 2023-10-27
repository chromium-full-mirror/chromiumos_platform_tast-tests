// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PowerCellularIdle,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Collect power metrics when device is in idle with cellular on and no UI",
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Contacts:       []string{"cros-cellular-core@google.com"},
		Attr:           []string{"group:cellular", "cellular_power", "cellular_unstable", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_unstable"},
		Timeout:        10 * time.Minute,
		Fixture:        "cellularPower",
	})
}

const modemEnableTime = 3 * time.Minute
const modemDisableTime = 1 * time.Minute

func PowerCellularIdle(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Test Disable / Enable .
	for i := 0; i < 2; i++ {
		s.Logf("Cellular Disable %d", i+1)
		if _, err := helper.Disable(ctx); err != nil {
			s.Fatalf("Disable failed on attempt %d: %s", i+1, err)
		}
		serviceCtx, serviceCancel := context.WithTimeout(ctx, 3*time.Second)
		defer serviceCancel()
		if _, err := helper.FindServiceForDevice(serviceCtx); err == nil {
			s.Fatal("Service found while Disabled")
		}

		//GoBigSleepLint: sleep to measure power when modem is disabled
		if err := testing.Sleep(ctx, modemDisableTime); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		s.Logf("Cellular Enable %d", i+1)
		if _, err := helper.Enable(ctx); err != nil {
			s.Fatalf("Enable failed on attempt %d: %s", i+1, err)
		}

		//GoBigSleepLint: sleep to measure power when modem is enabled and idle
		if err := testing.Sleep(ctx, modemEnableTime); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}
}
