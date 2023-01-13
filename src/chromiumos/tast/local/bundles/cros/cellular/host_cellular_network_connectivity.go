// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HostCellularNetworkConnectivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that host has network connectivity via cellular interface",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:      "cellular",
		Timeout:      4 * time.Minute,
	})
}

func HostCellularNetworkConnectivity(ctx context.Context, s *testing.State) {
	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create connected cellular.Helper (precondition): ", err)
	}

	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
