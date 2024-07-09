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
		Func:     DHCPNonASCIIParam,
		Desc:     "Verify DHCP negotiation when the server response contain non-ASCII parameters",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func DHCPNonASCIIParam(ctx context.Context, s *testing.State) {
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

	// There used to be a crash when the response contains parameters which is in
	// neither ASCII nor UTF-8. See https://crbug.com/217853.
	serverName := "\xff"
	dhcpOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)
	dhcpFields := dhcp.NewFieldMap(serverName)

	rules := []dhcp.HandlingRule{
		*dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcpFields, true /*shouldRespond*/),
		*dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcpFields, true, /*shouldRespond*/
			gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/),
	}
	rules[len(rules)-1].SetIsFinalHandler(true)

	if _, errs := dhcp.RunTestWithEnv(ctx, rt, rules, func(ctx context.Context) error {
		// There will be a 3-second delay in the test, for the first DHCPDISCOVER from
		// dhcpcd is very likely to be missed by the test DHCP server. However, this is
		// already the best thing we can do. We should not reconnect the device since
		// it is possible that dhcpcd already sent out the DISCOVER packet and thus the
		// test DHCP server will not respond to the DISCOVER packet after the
		// reconnection, and this will make the test flaky (b/332186871).
		return svc.WaitForConnectedOrError(ctx)
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
	}
}
