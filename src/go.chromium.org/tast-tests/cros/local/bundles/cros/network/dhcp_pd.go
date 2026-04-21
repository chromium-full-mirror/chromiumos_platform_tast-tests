// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dhcpd"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/radvd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type dhcppdTestParams struct {
	hasIPv4 bool
	hasARC  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPPD,
		Desc:     "Verify the shill behavior and routing semantics in a DHCPv6-PD environment",
		Contacts: []string{"cros-networking@google.com", "chenzikai@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Val: dhcppdTestParams{},
		}, {
			Name: "with_ipv4",
			Val: dhcppdTestParams{
				hasIPv4: true,
			},
		}, {
			Name: "with_arc",
			Val: dhcppdTestParams{
				hasARC: true,
			},
			ExtraSoftwareDeps: []string{"arc", "chrome"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted.ehide",
		}, {
			Name: "with_arc_with_ipv4",
			Val: dhcppdTestParams{
				hasIPv4: true,
				hasARC:  true,
			},
			ExtraSoftwareDeps: []string{"arc", "chrome"},
			ExtraHardwareDeps: arc.ArcAppHwDeps,
			Fixture:           "arcBooted.ehide",
		}},
	})
}

func DHCPPD(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hasIPv4 := s.Param().(dhcppdTestParams).hasIPv4
	hasARC := s.Param().(dhcppdTestParams).hasARC
	var a *arc.ARC
	if hasARC {
		a = s.FixtValue().(*arc.PreData).ARC
	}

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewResetVirtualnetHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewDumpARCOnFailureHook(a),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

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
	d := dhcpd.New(dhcpd.WithDHCPPD(subnet))
	if err := testEnv.Router.StartServer(ctx, "dhcpd", d); err != nil {
		s.Fatal("Failed to start dhcpd: ", err)
	}

	// Wait for service to be online.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 15*time.Second); err != nil {
		s.Fatal("Failed to wait for service online: ", err)
	}

	// Add route for return traffic in router with host as next hop.
	if err := d.AddDelegatedPrefixRouteToClient(ctx); err != nil {
		s.Fatal("Failed to add delegated prefix route: ", err)
	}

	// Verify topology in host.
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
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 15*time.Second); err != nil {
			s.Errorf("Network verification failed: %v is not reachable as user %s on host: %v", target, "chronos", err)
		}
	}

	if !hasARC {
		return
	}

	// Verify connectivity in ARC.
	vethName := testEnv.Router.VethOutName
	arcIfname, err := arcnet.GetARCInterfaceName(ctx, vethName)
	if err != nil {
		s.Fatalf("Failed to get ARC interface name corresponding to %s: %v", vethName, err)
	}

	// Check if testEnv prefix propagated into ARC, and log it for debugging.
	const addressPollTimeout = 10 * time.Second
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := a.Command(ctx, "/system/bin/ip", "-6", "addr", "show", "scope", "global", "dev", arcIfname).Output(testexec.DumpLogOnError)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.New("no global IPv6 address is configured")
		}
		testing.ContextLog(ctx, "ARC address information: ", string(out))
		return nil
	}, &testing.PollOptions{Timeout: addressPollTimeout}); err != nil {
		s.Fatalf("Failed to get global IPv6 address on %s in ARC: %v", arcIfname, err)
	}

	// ping virtual router address and virtual server address from ARC.
	for _, target := range pingAddrs {
		if err := arcnet.ExpectPingSuccess(ctx, a, arcIfname, target); err != nil {
			s.Errorf("Failed to ping %s from ARC over %q: %v", target, arcIfname, err)
		}
	}
}
