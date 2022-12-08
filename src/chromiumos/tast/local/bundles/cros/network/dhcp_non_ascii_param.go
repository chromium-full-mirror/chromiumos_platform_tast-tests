// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/network/dhcp"
	"chromiumos/tast/local/network/virtualnet"
	"chromiumos/tast/local/network/virtualnet/subnet"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPNonASCIIParam,
		Desc:     "Verify DHCP negotiation when the server response contain non-ASCII parameters",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
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

	subnetIP := subnet.IP.To4()
	gatewayIP := net.IPv4(subnetIP[0], subnetIP[1], subnetIP[2], 1)
	intendedIP := net.IPv4(subnetIP[0], subnetIP[1], subnetIP[2], 2)

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
		if err := svc.Reconnect(ctx); err != nil {
			return errors.Wrap(err, "failed to reconnect the service")
		}
		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for service connected")
		}
		return nil
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
	}
}
