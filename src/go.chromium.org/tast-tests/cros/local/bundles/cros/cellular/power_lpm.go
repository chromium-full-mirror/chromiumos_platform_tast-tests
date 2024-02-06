// Copyright 2024 The ChromiumOS Authors
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
		Func:           PowerLpm,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Collect power metrics when modem is in low power mode",
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Contacts:       []string{"cros-cellular-core@google.com"},
		Attr:           []string{"group:cellular", "cellular_power", "cellular_unstable", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_unstable"},
		Timeout:        10 * time.Minute,
		Fixture:        "cellularPower",
	})
}

const modemLowPowerModeTime = 3 * time.Minute

func PowerLpm(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	s.Log("Disable cellular to set modem to low power mode")
	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Disable failed: ", err)
	}
	defer helper.Enable(ctx)

	serviceCtx, serviceCancel := context.WithTimeout(ctx, 3*time.Second)
	defer serviceCancel()
	if _, err := helper.FindServiceForDevice(serviceCtx); err == nil {
		s.Fatal("Service found while Disabled")
	}

	s.Logf("Sleep for %d mins to collect modem lpm power metrics", modemLowPowerModeTime/time.Minute)
	//GoBigSleepLint: sleep to collect modem lpm power metrics
	if err := testing.Sleep(ctx, modemLowPowerModeTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
}
