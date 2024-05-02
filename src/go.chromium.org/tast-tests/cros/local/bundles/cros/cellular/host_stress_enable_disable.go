// Copyright 2022 The ChromiumOS Authors
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
		Func:           HostStressEnableDisable,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that host has network connectivity via cellular interface",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:        "cellularStressLocal",
		Timeout:        4 * time.Minute,
	})
}

func HostStressEnableDisable(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	stressTestHostIPConnectivity := func(ctx context.Context) error {
		for i := 1; i < 5; i++ {
			s.Logf("Test loop: %d", i)
			s.Log("Disable")
			if _, err := helper.Disable(ctx); err != nil {
				return errors.Wrap(err, "failed to disable modem")
			}
			s.Log("Enable")
			if _, err := helper.Enable(ctx); err != nil {
				return errors.Wrap(err, "failed to enable modem")
			}
			s.Log("Find Service")
			if _, err := helper.FindServiceForDevice(ctx); err != nil {
				return errors.Wrap(err, "failed to find default Service")
			}
			ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
			if err != nil {
				s.Fatal("Failed to read APN info: ", err)
			}
			s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)
			if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6); err != nil {
				return errors.Wrap(err, "failed connectivity test")
			}
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, stressTestHostIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
