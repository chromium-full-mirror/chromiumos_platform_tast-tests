// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillEnableAndConnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that Shill can enable, disable, connect, and disconnect to a Cellular Service",
		Contacts:       []string{"chromeos-cellular-team@google.com", "cros-network-health-team@google.com", "ejcaruso@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_sim_active"},
		Fixture:        "cellularStressLocal",
		HardwareDeps:   hwdep.D(hwdep.Cellular()),
		Timeout:        10 * time.Minute,
	})
}

func ShillEnableAndConnect(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Disable AutoConnect so that enable does not connect.
	ctxForAutoConnectCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cellular.AutoConnectCleanupTime)
	defer cancel()
	if wasAutoConnect, err := helper.SetServiceAutoConnect(ctx, false); err != nil {
		s.Fatal("Failed to disable AutoConnect: ", err)
	} else if wasAutoConnect {
		defer func(ctx context.Context) {
			if _, err := helper.SetServiceAutoConnect(ctx, true); err != nil {
				s.Fatal("Failed to re-enable AutoConnect: ", err)
			}
		}(ctxForAutoConnectCleanUp)
	}

	perfValues := perf.NewValues()

	// Test Disable / Enable / Connect / Disconnect.
	// Run the test a second time to test Disable after Connect/Disconnect.
	// Run the test a third time to help test against flakiness.
	for i := 0; i < 3; i++ {
		s.Logf("Disable %d", i)
		disableTime, err := helper.Disable(ctx)
		if err != nil {
			s.Fatalf("Disable failed on attempt %d: %s", i, err)
		}
		perfValues.Append(perf.Metric{
			Name:      "cellular_disable_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, disableTime.Seconds())
		s.Logf("Enable %d", i)
		enableTime, err := helper.Enable(ctx)
		if err != nil {
			s.Fatalf("Enable failed on attempt %d: %s", i, err)
		}
		perfValues.Append(perf.Metric{
			Name:      "cellular_enable_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, enableTime.Seconds())
		s.Logf("Connect %d", i)
		connectTime, err := helper.ConnectToDefault(ctx)
		if err != nil {
			s.Fatalf("Connect failed on attempt %d: %s", i, err)
		}
		perfValues.Append(perf.Metric{
			Name:      "cellular_connect_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, connectTime.Seconds())
		s.Logf("Disconnect %d", i)
		disconnectTime, err := helper.Disconnect(ctx)
		if err != nil {
			s.Fatalf("Disconnect failed on attempt %d: %s", i, err)
		}
		perfValues.Append(perf.Metric{
			Name:      "cellular_disconnect_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, disconnectTime.Seconds())
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}

	// Test that Disconnect fails while not connected.
	if _, err := helper.Disconnect(ctx); err == nil {
		s.Fatal("Disconnect succeeded while disconnected: ", err)
	}

	s.Log("Reconnect")
	if _, err := helper.ConnectToDefault(ctx); err != nil {
		s.Fatal("Reconnect failed: ", err)
	}

	// Test Disable while connected.
	s.Log("Disable Cellular while Connected")
	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Disable failed: ", err)
	}

	s.Log("Ensure no Cellular Service while disabled")
	serviceCtx, serviceCancel := context.WithTimeout(ctx, 3*time.Second)
	defer serviceCancel()
	if _, err := helper.FindServiceForDevice(serviceCtx); err == nil {
		s.Fatal("Service found while Disabled")
	}

	s.Log("Final Enable")
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Final Enable failed: ", err)
	}
}
