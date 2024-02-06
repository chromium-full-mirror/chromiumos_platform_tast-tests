// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillEnableDisable,
		Desc:         "Verifies that Shill can enable, and disable a cellular device",
		Contacts:     []string{"chromeos-cellular-team@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq"},
		HardwareDeps: hwdep.D(hwdep.Cellular()),
		Timeout:      5 * time.Minute,
		Fixture:      "cellularAutoconnectLocal",
	})
}

func ShillEnableDisable(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	perfValues := perf.NewValues()

	// Test Disable / Enable .
	// Run the test a second time to test Disable after Enable.
	for i := 0; i < 2; i++ {
		s.Logf("Disable %d", i+1)
		disableTime, err := helper.Disable(ctx)
		if err != nil {
			s.Fatalf("Disable failed on attempt %d: %s", i+1, err)
		}
		appendPerfMetric(perfValues, "cellular_disable_time", disableTime)
		serviceCtx, serviceCancel := context.WithTimeout(ctx, 3*time.Second)
		defer serviceCancel()
		if _, err := helper.FindServiceForDevice(serviceCtx); err == nil {
			s.Fatal("Service found while Disabled")
		}

		s.Logf("Enable %d", i+1)
		enableTime, err := helper.Enable(ctx)
		if err != nil {
			s.Fatalf("Enable failed on attempt %d: %s", i+1, err)
		}
		appendPerfMetric(perfValues, "cellular_enable_time", enableTime)
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func appendPerfMetric(perfValues *perf.Values, name string, t time.Duration) {
	perfValues.Append(perf.Metric{
		Name:      name,
		Unit:      "seconds",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, t.Seconds())
}
