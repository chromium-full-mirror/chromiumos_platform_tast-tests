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
		Func:     DHCPTwoServersConflict,
		Desc:     "Verify the DHCP behavior in the case of two DHCP servers which offers an invalid address",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// DHCPTwoServersConflict verifies the DHCP negotiation in the case that there
// are two DHCP servers on the link, one of the server response to DISCOVER with
// an invalid address. Then when we request this address to the servers, one
// server rejects it with NAK while the other one accepts it with ACK. This test
// mainly verifies two behaviors in this case: 1) the client should broadcast a
// DECLINE to reject the invalid lease; 2) the client should ignore the
// following DHCP offers if they still contain this invalid address. See
// https://crbug.com/384897.
func DHCPTwoServersConflict(ctx context.Context, s *testing.State) {
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

	badDHCPOpts := dhcp.NewOptionMap(gatewayIP, gatewayIP)
	goodDHCPOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)
	rules := []dhcp.HandlingRule{
		// DHCP server offers us an invalid address (the address of server itself).
		*dhcp.NewRespondToDiscovery(gatewayIP.String(), gatewayIP.String(),
			badDHCPOpts, dhcp.FieldMap{}, true /*shouldRespond*/),
		// The client should send REQUEST to the server for the invalid address, and
		// there are two servers on the link, the first response rejects the request
		// and the second response accepts the request.
		*dhcp.NewRejectAndRespondToRequest(gatewayIP.String(), gatewayIP.String(),
			badDHCPOpts, dhcp.FieldMap{}, true),

		// The client should send out a DECLINE packet to the server since the
		// offered address is not valid.
		*dhcp.NewAcceptDecline(gatewayIP.String(), badDHCPOpts, dhcp.FieldMap{}),

		// The client should start the negotiation from the beginning. The server is
		// still providing the invalid address, and the client should just ignore it.
		*dhcp.NewRespondToDiscovery(gatewayIP.String(), gatewayIP.String(),
			badDHCPOpts, dhcp.FieldMap{}, true /*shouldRespond*/),

		// The final round of negotiation works normally.
		*dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
			goodDHCPOpts, dhcp.FieldMap{}, true /*shouldRespond*/),
		*dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
			goodDHCPOpts, dhcp.FieldMap{}, true, /*shouldRespond*/
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
