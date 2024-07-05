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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	arcutil "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ARCVPNDatapath,
		Desc:     "Ensure that datapath works properly when a ARC VPN is connected",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "arcBooted",
		SoftwareDeps: []string{"arc"},
	})
}

func ARCVPNDatapath(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
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

	// Verify physicalEnv can be reached by IPv6 before connecting VPN.
	physicalAddrs, err := physicalEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to addrs from physical env: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos", 10*time.Second); err != nil {
		s.Fatal("Cannot reach physical env by IPv6: ", err)
	}

	opts := []vpn.Option{
		vpn.WithIPType(vpn.IPTypeIPv4),
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

	// Start a DNS server. This server is only reachable via VPN.
	const domain = "domain.test"
	const vpnResolveResult = "203.0.113.33"
	if err := vpnEnv.StartServer(ctx, "dnsmasq", dnsmasq.New(
		dnsmasq.WithResolveHost(domain, net.ParseIP(vpnResolveResult)),
		dnsmasq.WithInterface(server.OverlayIfname),
	)); err != nil {
		s.Fatal("Failed to start dnsmasq on vpn server: ", err)
	}

	// The resolve result on physical network. By default, it should be the router
	// address for a network created by vpn.CreateNetworkTopology().
	physicalResolveResult := func() string {
		routerAddrs, err := networkEnv.Router.GetVethInAddrs(ctx)
		if err != nil {
			s.Fatal("Failed to get router addrs: ", err)
		}
		return routerAddrs.IPv4Addr.String()
	}()

	// Install and start the test app.
	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to set up ARC VPN test app")
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
		s.Fatal("Failed to wait for VPN service connected in shill")
	}

	s.Log("VPN connected in shill. Verifying routing")

	privateEnv, err := networkEnv.CreatePrivateEnv(ctx, server, vpnEnv)
	if err != nil {
		s.Fatal("Failed to create VPN private env: ", err)
	}

	privateEnvIPs, err := privateEnv.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get IPs in private env: ", err)
	}

	type ipAndRole struct {
		ip   string
		role string
	}
	reachableIPs := []ipAndRole{
		{server.OverlayIPv4, "server IPv4"},
		{privateEnvIPs.IPv4Addr.String(), "private IPv4"},
	}
	for _, ip := range reachableIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, ip.ip, "chronos", 10*time.Second); err != nil {
			s.Fatalf("Failed to ping %s %s: %v", ip.role, ip.ip, err)
		}
	}

	// IPv6 should be blocked.
	if err := ping.ExpectPingFailure(ctx, physicalAddrs.IPv6Addrs[0].String(), "chronos"); err != nil {
		s.Fatal("IPv6 ping should fail: ", err)
	}

	// Verify that system traffic is still able to reach physical network.
	physicalIPs := []ipAndRole{
		{physicalAddrs.IPv4Addr.String(), "physical IPv4"},
		{physicalAddrs.IPv6Addrs[0].String(), "physical IPv6"},
	}
	for _, ip := range physicalIPs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, ip.ip, "root", 5*time.Second); err != nil {
			s.Fatalf("Failed to ping %s %s as root: %v", ip.role, ip.ip, err)
		}
	}

	s.Log("VPN routing verified. Verifying DNS")

	// Verify that the private domain will be resolved to different addresses for
	// user traffic and system traffic.
	if err := dns.VerifyDNSResolve(ctx, "chronos", domain, true, vpnResolveResult); err != nil {
		s.Fatalf("Failed to resolve %s as chronos: %v", domain, err)
	}
	if err := dns.VerifyDNSResolve(ctx, "root", domain, true, physicalResolveResult); err != nil {
		s.Fatalf("Failed to resolve %s as root: %v", domain, err)
	}
}
