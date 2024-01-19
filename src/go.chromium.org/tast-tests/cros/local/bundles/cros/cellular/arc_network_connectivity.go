// Copyright 2022 The ChromiumOS Authors
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
		Func:           ArcNetworkConnectivity,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that Arc has network connectivity via cellular interface",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_sim_active"},
		SoftwareDeps:   []string{"chrome", "vm_host"},
		Fixture:        "cellularArcBooted",
		Timeout:        4 * time.Minute,
	})
}

func ArcNetworkConnectivity(ctx context.Context, s *testing.State) {
	arc := s.FixtValue().(*cellular.FixtData).ARC

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	verifyIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyArcIPConnectivity(ctx, ipv4, ipv6, arc); err != nil {
			return err
		}
		return nil
	}
	if err := helper.RunTestOnCellularInterface(ctx, verifyIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
