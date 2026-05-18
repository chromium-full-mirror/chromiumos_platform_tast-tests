// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	arcutil "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type arcVPNSplitRoutingTestCase int

// In this test, we will set up two virtualnet envs (a VPN one and a physical
// one) connecting to the router. The VPN env is reachable via the VPN while the
// physical one is not.
const (
	// Set up VPN routes to only include VPN env for both IP families. Both VPN
	// and physical env should be reachable as chronos user.
	arcVPNSplitRoutingTestCaseIncludedRoutes arcVPNSplitRoutingTestCase = iota
	// Set up VPN routes to only include VPN env for both IP families. Verify
	// that the split-routing setup works for IPv4 and all IPv6 traffic should
	// be blocked. This is a special test case for ARC R.
	arcVPNSplitRoutingTestCaseIncludedRoutesRVC
	// Set up VPN routes to only exclude physical env for both IP families. Both
	// VPN and physical env should be reachable as chronos user.
	arcVPNSplitRoutingTestCaseExcludedRoutes
	// Set up VPN routes to have default routes for both IP families. Physical env
	// shouldn't be reachable as chronos user. Technically this should belong to
	// this test, but since for a ARC VPN the handling is different for a
	// dual-stack VPN and a IPv4-only VPN, we want to cover both of them, and the
	// IPv4-only VPN is covered by the ARCVPNDatapath test.
	// TODO(jiejiang): Consider moving this test case to ARCVPNDatapath.
	arcVPNSplitRoutingTestCaseDefaultRoutes
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ARCVPNSplitRouting,
		Desc:     "Verify routing for a split-routing ARCVPN",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"arc"},
		HardwareDeps: arc.ArcAppHwDeps,
		Params: []testing.Param{
			{
				Name: "included",
				Val:  arcVPNSplitRoutingTestCaseIncludedRoutes,
				// IPv6 split routing is only supported on ARC T+.
				ExtraSoftwareDeps: []string{"no_android_r"},
			}, {
				Name: "excluded",
				Val:  arcVPNSplitRoutingTestCaseExcludedRoutes,
				// IPv6 split routing is only supported on ARC T+.
				ExtraSoftwareDeps: []string{"no_android_r"},
			}, {
				Name: "default",
				Val:  arcVPNSplitRoutingTestCaseDefaultRoutes,
			},
		},
	})
}

func ARCVPNSplitRouting(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

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

	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()

	// Use Server1 for setting up VPN server, and Server2 for verify physical
	// traffic.
	vpnEnv := networkEnv.Server1
	physicalEnv := networkEnv.Server2

	createIPv4Subnet := func(cidr string) *subnet.IPv4Subnet {
		ret, err := subnet.FromIPv4CIDR(cidr)
		if err != nil {
			s.Fatal("Failed to create IPv4 subnet from CIDR string: ", err)
		}
		return ret
	}

	createIPv6Subnet := func(cidr string) *subnet.IPv6Subnet {
		ret, err := subnet.FromIPv6CIDR(cidr)
		if err != nil {
			s.Fatal("Failed to create IPv6 subnet from CIDR string: ", err)
		}
		return ret
	}

	// Subnets for the VPN overlay. Note that the ARC VPN in the test doesn't
	// really support tunneling IPv6 traffic -- it will only claim to the Android
	// system that it does. Since we don't support IPv6 ARC VPN in the host this
	// should be enough in this test.
	vpnIPv4Subnet := createIPv4Subnet("10.11.12.0/24")
	vpnIPv6Subnet := createIPv6Subnet("fddf::/64")

	// Subnets for physical env.
	physicalAddrs, err := physicalEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to addrs from physical env: ", err)
	}
	physicalIPv4Subnet := &net.IPNet{
		IP:   physicalAddrs.IPv4Addr,
		Mask: net.CIDRMask(24, 32),
	}
	physicalIPv6Subnet := &net.IPNet{
		IP:   physicalAddrs.IPv6Addrs[0],
		Mask: net.CIDRMask(64, 128),
	}

	// Set up an additional Env which is reachable via physical but not
	// reachable via VPN. The IPv6 addr of this Env will be included in the VPN
	// in all test cases (explicitly or implicitly), and can be used for
	// verifying the IPv6 blackhole route for this subnet is set up properly.
	physicalEnv2, err := virtualnet.CreateEnv(ctx, "physical2")
	if err != nil {
		s.Fatal("Failed to create the second physical Env: ", err)
	}
	// Note: IPv4 subnet does not matter here, but ConnectToRouter() requires
	// one.
	physicalIPv4Subnet2 := createIPv4Subnet("10.12.14.0/24")
	physicalIPv6Subnet2 := createIPv6Subnet("fdff::/64")
	if err := physicalEnv2.ConnectToRouter(ctx, networkEnv.Router, physicalIPv4Subnet2, physicalIPv6Subnet2); err != nil {
		s.Fatal("Failed to connect the second physical Env to router: ", err)
	}
	physicalIPv6Addr2 := physicalIPv6Subnet2.GetAddrEndWith(2)
	if err := ping.ExpectPingSuccessWithTimeout(ctx, physicalIPv6Addr2.String(), "chronos", 5*time.Second); err != nil {
		s.Fatal("Failed to verify IPv6 connectivity to the second physical Env: ", err)
	}

	// "Subnets" for default routes.
	defaultIPv4Subnet := &net.IPNet{
		IP:   net.IPv4zero,
		Mask: net.CIDRMask(0, 32),
	}
	defaultIPv6Subnet := &net.IPNet{
		IP:   net.IPv6zero,
		Mask: net.CIDRMask(0, 128),
	}

	opts := []vpn.Option{
		vpn.WithIPType(vpn.IPTypeIPv4AndIPv6),
		vpn.WithIPv4Subnet(vpnIPv4Subnet),
		vpn.WithIPv6Subnet(vpnIPv6Subnet),
	}

	tc := s.Param().(arcVPNSplitRoutingTestCase)
	switch tc {
	case arcVPNSplitRoutingTestCaseIncludedRoutes:
		opts = append(opts, vpn.WithIPv4IncludedRoute(&vpnIPv4Subnet.IPNet))
		opts = append(opts, vpn.WithIPv6IncludedRoute(&vpnIPv6Subnet.IPNet))
		opts = append(opts, vpn.WithIPv6IncludedRoute(&physicalIPv6Subnet2.IPNet))
	case arcVPNSplitRoutingTestCaseExcludedRoutes:
		opts = append(opts, vpn.WithExcludedRoute(physicalIPv4Subnet))
		opts = append(opts, vpn.WithExcludedRoute(physicalIPv6Subnet))
	case arcVPNSplitRoutingTestCaseDefaultRoutes:
		opts = append(opts, vpn.WithIPv4IncludedRoute(defaultIPv4Subnet))
		opts = append(opts, vpn.WithIPv4IncludedRoute(defaultIPv6Subnet))
	}

	config := vpn.NewConfig(vpn.TypeToyVPNServer, opts...)
	server, err := vpn.StartServerWithConfig(ctx, vpnEnv, config)
	if err != nil {
		s.Fatal("Failed to create VPN server: ", err)
	}
	defer func() {
		if err := server.Exit(cleanupCtx); err != nil {
			s.Error("Failed to stop VPN server: ", err)
		}
	}()

	// Install and start the test app.
	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to set up ARC VPN test app: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	ifname, err := arcutil.GetARCInterfaceName(ctx, networkEnv.Router.VethOutName)
	if err != nil {
		s.Fatal("Failed to get the interface name inside ARC: ", err)
	}

	testing.ContextLog(ctx, "Starting ArcVpnTest app")
	if err := arcvpn.StartARCVPNWithToyServer(ctx, a, ifname, server); err != nil {
		s.Fatal("Failed to start ARCVPN: ", err)
	}

	// Make sure our test app is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	// Make sure VPN service is connected in shill.
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		props := map[string]interface{}{
			shillconst.ServicePropertyType:  shillconst.TypeVPN,
			shillconst.ServicePropertyState: shillconst.ServiceStateOnline,
		}
		if _, err := manager.FindMatchingService(ctx, props); err != nil {
			if err.Error() == shillconst.ErrorMatchingServiceNotFound {
				return errors.New("failed to find online VPN service in shill")
			}
			return testing.PollBreak(err)
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to wait for VPN service connected in shill: ", err)
	}

	s.Log("VPN connected in shill. Verifying routing")

	type ipAndRole struct {
		ip   string
		role string
	}
	var reachableIPs []ipAndRole
	unreachableIPs := []ipAndRole{
		// For an ARC VPN, all destinations in the included routes will be
		// blocked on the host, so this should not be reachable in all cases.
		ipAndRole{physicalIPv6Addr2.String(), "VPN included IPv6"},
	}
	switch tc {
	case arcVPNSplitRoutingTestCaseIncludedRoutes:
		fallthrough
	case arcVPNSplitRoutingTestCaseExcludedRoutes:
		reachableIPs = append(reachableIPs, ipAndRole{physicalAddrs.IPv4Addr.String(), "physical IPv4"})
		reachableIPs = append(reachableIPs, ipAndRole{server.OverlayIPv4, "VPN overlay IPv4"})
		// VPN overlay IPv6 won't be reachable. Check it here as a confidence check for our setup.
		unreachableIPs = append(unreachableIPs, ipAndRole{server.OverlayIPv6, "VPN overlay IPv6"})

		// The physical IPv6 addr is not included in the routes for VPN. On ARC
		// T+, it should be reachable; while on ARC R, it should be blocked.
		if tc == arcVPNSplitRoutingTestCaseIncludedRoutesRVC {
			unreachableIPs = append(unreachableIPs, ipAndRole{physicalAddrs.IPv6Addrs[0].String(), "physical IPv6"})
		} else {
			reachableIPs = append(reachableIPs, ipAndRole{physicalAddrs.IPv6Addrs[0].String(), "physical IPv6"})
		}

		// Check the VPN underlay IPv6 to verify if blackhole route is applied properly.
		vpnUnderlayAddrs, err := vpnEnv.GetVethInAddrs(ctx)
		if err != nil {
			s.Fatal("Failed to get addrs in VPN env: ", err)
		}
		if tc == arcVPNSplitRoutingTestCaseIncludedRoutes {
			reachableIPs = append(reachableIPs, ipAndRole{vpnUnderlayAddrs.IPv6Addrs[0].String(), "VPN underlay IPv6"})
		} else {
			unreachableIPs = append(unreachableIPs, ipAndRole{vpnUnderlayAddrs.IPv6Addrs[0].String(), "VPN underlay IPv6"})
		}
	case arcVPNSplitRoutingTestCaseDefaultRoutes:
		unreachableIPs = append(unreachableIPs, ipAndRole{physicalAddrs.IPv4Addr.String(), "physical IPv4"})
		unreachableIPs = append(unreachableIPs, ipAndRole{physicalAddrs.IPv6Addrs[0].String(), "physical IPv6"})
		reachableIPs = append(reachableIPs, ipAndRole{server.OverlayIPv4, "VPN overlay IPv4"})
		// VPN overlay IPv6 won't be reachable. Check it here as a confidence check for our setup.
		unreachableIPs = append(unreachableIPs, ipAndRole{server.OverlayIPv6, "VPN overlay IPv6"})
	}

	for _, ip := range reachableIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, ip.ip, "chronos", 5*time.Second); err != nil {
			s.Fatalf("Failed to ping %s %s: %v", ip.role, ip.ip, err)
		}
	}
	for _, ip := range unreachableIPs {
		if err := ping.ExpectPingFailure(ctx, ip.ip, "chronos"); err != nil {
			s.Fatalf("Expect failure but ping succeeded %s %s: %v", ip.role, ip.ip, err)
		}
	}
}
