// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPTwoServersNAK,
		Desc:     "Verify the DHCP behavior in the case of two DHCP servers",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// DHCPTwoServersNAK verifies the DHCP negotiation in the case that there are
// two DHCP servers on the link, one of them accepts our request while another
// one rejects it. We expect that DUT should still take the lease. See
// https://crbug.com/384897.
func DHCPTwoServersNAK(ctx context.Context, s *testing.State) {
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

	dhcpOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)

	// Two cases simulate that DUT receives ACK at first or NAK at first.
	tcs := []struct {
		name     string
		nakFirst bool
	}{{
		name:     "NAK first",
		nakFirst: true,
	}, {
		name:     "ACK first",
		nakFirst: false,
	}}

	for _, tc := range tcs {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			discoverRule := dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
				dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/)
			requestRule := dhcp.NewRejectAndRespondToRequest(intendedIP.String(), gatewayIP.String(),
				dhcpOpts, dhcp.FieldMap{}, tc.nakFirst)
			requestRule.SetIsFinalHandler(true)

			if _, errs := dhcp.RunTestWithEnv(ctx, rt, []dhcp.HandlingRule{*discoverRule, *requestRule}, func(ctx context.Context) error {
				if err := svc.Reconnect(ctx); err != nil {
					return errors.Wrap(err, "failed to reconnect the service")
				}
				if err := svc.WaitForConnectedOrError(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for service connected")
				}

				// GoBigSleepLint: Sleep for a while and verify again to make sure the
				// received NAK does not have any effect.
				testing.Sleep(ctx, 3*time.Second)
				if connected, err := svc.IsConnected(ctx); err != nil {
					return errors.Wrap(err, "failed to get connected status")
				} else if !connected {
					return errors.New("service is not connected after sleep")
				}
				return nil
			}); len(errs) > 0 {
				for _, err := range errs {
					s.Error("Failed to verify DHCP negotiation: ", err)
				}
			}
		})
	}
}
