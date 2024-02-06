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
		Func:           PowerIdle,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Collect power metrics when device is idle with cellular on",
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Contacts:       []string{"cros-cellular-core@google.com"},
		Attr:           []string{"group:cellular", "cellular_power", "cellular_unstable", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_unstable"},
		Timeout:        10 * time.Minute,
		Fixture:        "cellularPower",
	})
}

const modemIdleTime = 3 * time.Minute

func PowerIdle(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	s.Log("Toggle Cellular")
	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Disable failed: ", err)
	}
	defer helper.Enable(ctx)

	serviceCtx, serviceCancel := context.WithTimeout(ctx, 3*time.Second)
	defer serviceCancel()
	if _, err := helper.FindServiceForDevice(serviceCtx); err == nil {
		s.Fatal("Service found while Disabled")
	}
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Enable failed: ", err)
	}

	//GoBigSleepLint: sleep to measure power when modem is enabled and idle
	if err := testing.Sleep(ctx, modemIdleTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
}
