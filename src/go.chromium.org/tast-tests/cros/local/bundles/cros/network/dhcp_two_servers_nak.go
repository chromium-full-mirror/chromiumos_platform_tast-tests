// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type twoServersNAKTestParam struct {
	nakFirst      bool
	dhcpcdVersion testhooks.DhcpcdVersion
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPTwoServersNAK,
		Desc:     "Verify the DHCP behavior in the case of two DHCP servers",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:release-health", "release-health_network"},
		// The param value represents whether NAK comes first.
		Params: []testing.Param{{
			Name: "nak_first",
			Val: twoServersNAKTestParam{
				nakFirst:      true,
				dhcpcdVersion: testhooks.Dhcpcd7,
			},
		}, {
			Name: "ack_first",
			Val: twoServersNAKTestParam{
				nakFirst:      false,
				dhcpcdVersion: testhooks.Dhcpcd7,
			},
		}, {
			Name: "nak_first_dhcpcd10",
			Val: twoServersNAKTestParam{
				nakFirst:      true,
				dhcpcdVersion: testhooks.Dhcpcd10,
			},
		}, {
			Name: "ack_first_dhcpcd10",
			Val: twoServersNAKTestParam{
				nakFirst:      false,
				dhcpcdVersion: testhooks.Dhcpcd10,
			},
		}},
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

	param := s.Param().(twoServersNAKTestParam)
	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewSetDhcpcdVersionHook(param.dhcpcdVersion),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Prepare the environment.
	pool := subnet.NewPool()
	svc, rt, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnet.EnvOptions{})
	if err != nil {
		s.Fatal("Failed to create router env: ", err)
	}
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

	dhcpOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)

	discoverRule := dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/)
	requestRule := dhcp.NewRejectAndRespondToRequest(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, param.nakFirst)
	requestRule.SetIsFinalHandler(true)

	if _, errs := dhcp.RunTestWithEnv(ctx, rt, []dhcp.HandlingRule{*discoverRule, *requestRule}, func(ctx context.Context) error {
		// There will be a 3-second delay in the test, for the first DHCPDISCOVER from
		// dhcpcd is very likely to be missed by the test DHCP server. However, this is
		// already the best thing we can do. We should not reconnect the device since
		// it is possible that dhcpcd already sent out the DISCOVER packet and thus the
		// test DHCP server will not respond to the DISCOVER packet after the
		// reconnection, and this will make the test flaky (b/332186871).
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
}
