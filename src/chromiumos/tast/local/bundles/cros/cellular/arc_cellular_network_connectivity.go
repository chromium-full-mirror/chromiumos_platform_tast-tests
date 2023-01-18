// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcCellularNetworkConnectivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that Arc has network connectivity via cellular interface",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		SoftwareDeps: []string{"chrome", "vm_host"},
		Fixture:      "cellular",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func ArcCellularNetworkConnectivity(ctx context.Context, s *testing.State) {
	defer upstart.RestartJob(ctx, "ui")
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create connected cellular.Helper (precondition): ", err)
	}

	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	// StartARC
	func() {
		cr, err := chrome.New(ctx, chrome.ARCEnabled())
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(ctx)
		a, err := arc.New(ctx, s.OutDir())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(ctx)
	}()

	verifyIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivity(ctx, arc.BootstrapCommand, ipv4, ipv6, "/system/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}
	if err := helper.RunTestOnCellularInterface(ctx, verifyIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
