// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPClientOptions,
		Desc:     "Verify the DHCP options in the DHCP packets sent out by the client",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DHCPClientOptions(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Prepare the environment.
	pool := subnet.NewPool()
	svc, rt, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnet.EnvOptions{})
	defer func() {
		if err := rt.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up router: ", err)
		}
	}()
	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	gatewayIP := subnet.GetAddrEndWith(1)
	intendedIP := subnet.GetAddrEndWith(2)

	// Install gateway address and routes.
	if err := rt.ConfigureInterface(ctx, rt.VethInName, gatewayIP, subnet); err != nil {
		s.Fatal("Failed to install address on router: ", err)
	}

	// Perform the DHCP negotiation.
	dhcpOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)
	rules := []dhcp.HandlingRule{
		*dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/),
		*dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcp.FieldMap{}, true, /*shouldRespond*/
			gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/),
	}
	rules[len(rules)-1].SetIsFinalHandler(true)
	dhcpServer, errs := dhcp.RunTestWithEnv(ctx, rt, rules, func(ctx context.Context) error {
		return svc.WaitForConnectedOrError(ctx)
	})
	if len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
		return
	}

	// Check if option values in the packets are expected.
	const expectedVendorID = "chromeos"
	for _, p := range dhcpServer.Packets() {
		if p.VendorID() != expectedVendorID {
			s.Errorf("Unexpected Vendor ID option value in DHCP packet: got %s, want %s, packet is %s", p.VendorID(), expectedVendorID, p)
		}
	}
}
