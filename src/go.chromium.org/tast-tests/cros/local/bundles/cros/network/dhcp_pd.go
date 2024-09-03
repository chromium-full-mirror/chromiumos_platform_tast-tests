// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dhcpd"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/radvd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dhcppdTestParams struct {
	hasIPv4 bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPPD,
		Desc:     "Verify the shill behavior and routing semantics in a DHCPv6-PD environment",
		Contacts: []string{"cros-networking@google.com", "taoyl@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Val: dhcppdTestParams{},
		}, {
			Name: "with_ipv4",
			Val: dhcppdTestParams{
				hasIPv4: true,
			},
		}},
	})
}

func DHCPPD(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewResetVirtualnetHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	hasIPv4 := s.Param().(dhcppdTestParams).hasIPv4

	// Set up test topology.
	testEnv := routing.NewSimpleNetworkEnv(hasIPv4, false, hasIPv4, false)
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	subnet, err := testEnv.Pool.AllocNextSlash63IPv6Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	if err := testEnv.Router.StartServer(ctx, "radvd", radvd.New(nil, []string{"fd00::1"})); err != nil {
		s.Fatal("Failed to start radvd: ", err)
	}
	if err := testEnv.Router.StartServer(ctx, "dhcpd", dhcpd.New(dhcpd.WithDHCPPD(subnet))); err != nil {
		s.Fatal("Failed to start dhcpd: ", err)
	}

	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for service online: ", err)
	}
	routerAddrs, err := testEnv.Router.WaitForVethInAddrs(ctx, hasIPv4, true /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get inner addrs from router env: ", err)
	}
	serverAddrs, err := testEnv.Server.WaitForVethInAddrs(ctx, hasIPv4, true /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get inner addrs from server env: ", err)
	}
	var pingAddrs []string
	for _, ip := range serverAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}
	for _, ip := range routerAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}
	if hasIPv4 {
		pingAddrs = append(pingAddrs, routerAddrs.IPv4Addr.String())
		pingAddrs = append(pingAddrs, serverAddrs.IPv4Addr.String())
	}
	for _, target := range pingAddrs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			s.Errorf("Network verification failed: %v is not reachable as user %s on host: %v", target, "chronos", err)
		}
	}

	// TODO(b/350884946): Verify connectivity in VMs.
}
