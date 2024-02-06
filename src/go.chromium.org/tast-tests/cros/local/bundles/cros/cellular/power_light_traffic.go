// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PowerLightTraffic,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Collect power metrics when modem has periodic ping traffic",
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Contacts:       []string{"cros-cellular-core@google.com", "rmao@google.com"},
		Attr:           []string{"group:cellular", "cellular_power", "cellular_unstable", "cellular_sim_active", "group:cellular_crosbolt", "cellular_crosbolt_unstable"},
		Timeout:        10 * time.Minute,
		Fixture:        "cellularPower",
	})
}

const (
	lightTrafficIterations = 9
	lightTrafficInterval   = 20 * time.Second
)

func PowerLightTraffic(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	triggerLightTraffic := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	for i := 0; i < lightTrafficIterations; i++ {
		if err := helper.RunTestOnCellularInterface(ctx, triggerLightTraffic); err != nil {
			s.Fatal("Failed to run test on cellular interface: ", err)
		}
		// GoBigSleepLint: sleep between data traffic.
		if err := testing.Sleep(ctx, lightTrafficInterval); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}
}
