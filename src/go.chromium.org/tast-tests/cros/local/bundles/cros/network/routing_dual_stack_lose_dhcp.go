// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/guestos"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type lostDHCPTestCase int

const (
	loseDHCPTestCaseARC lostDHCPTestCase = iota
	lostDHCPTestCaseCrostini
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     RoutingDualStackLoseDHCP,
		Desc:     "Verify the shill behavior in the scenario that DHCP fails to renew on a dual-stack network",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:network", "network_platform"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Name:              "arc",
			Val:               loseDHCPTestCaseARC,
			Fixture:           "arcBooted",
			ExtraSoftwareDeps: []string{"arc"},
		}, {
			Name:              "crostini",
			Val:               lostDHCPTestCaseCrostini,
			Fixture:           "crostiniBullseye",
			ExtraSoftwareDeps: []string{"vm_host"},
		}},
	})
}

func RoutingDualStackLoseDHCP(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testEnv := routing.NewTestEnv()
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	testNetworkOpts := virtualnet.EnvOptions{
		Priority:   routing.HighPriority,
		NameSuffix: routing.TestSuffix,
		EnableDHCP: false,
		RAServer:   true,
	}
	if err := testEnv.CreateNetworkEnvForTest(ctx, testNetworkOpts); err != nil {
		s.Fatal("Failed to create network for test: ", err)
	}

	subnet, err := testEnv.Pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	// Set up a DHCP server with our dhcp package. Do not use dnsmasq because the
	// dnsmasq will force the lease time no shorter than 2 minutes, while we can
	// do 20 seconds with our own server.
	gatewayIP := subnet.GetAddrEndWith(1)
	intendedIP := subnet.GetAddrEndWith(2)

	if err := testEnv.TestRouter.ConfigureInterface(ctx, testEnv.TestRouter.VethInName, gatewayIP, subnet); err != nil {
		s.Fatal("Failed to install address on router: ", err)
	}

	const leaseTime = 20 * time.Second
	dhcpOpts := dhcp.NewOptionMap(
		gatewayIP, intendedIP,
		dhcp.WithOptionLeaseTime(uint32(leaseTime.Seconds())),
	)

	rules := []dhcp.HandlingRule{
		*dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/),
		*dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
			dhcpOpts, dhcp.FieldMap{}, true, /*shouldRespond*/
			gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/),
	}
	rules[len(rules)-1].SetIsFinalHandler(true)

	if _, errs := dhcp.RunTestWithEnv(ctx, testEnv.TestRouter, rules, func(ctx context.Context) error {
		if err := testEnv.WaitForServiceOnline(ctx, testEnv.TestService); err != nil {
			return errors.Wrap(err, "failed to wait for test service online")
		}
		if errs := testEnv.VerifyTestNetwork(ctx, routing.VerifyOptions{
			IPv4:      true,
			IPv6:      true,
			IsPrimary: true,
			Timeout:   10 * time.Second,
		}); len(errs) != 0 {
			return errors.Wrap(errs[0], "failed to verify test network")
		}
		return nil
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
		return
	}

	// After lease expired, shill will restart the DHCP negotiation, and when it
	// fails this time, IPv4 failure event will be triggered in shill.
	const timeout = leaseTime + routing.DHCPTimeout
	testing.ContextLogf(ctx, "Waiting %s for DHCP timeout on test network", timeout)
	// GoBigSleepLint: sleep to let the DHCP timeout
	testing.Sleep(ctx, timeout)

	// Check that IPv4 address has disappeared from the IPConfig.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ipconfigs, err := testEnv.TestService.GetIPConfigs(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get IPConfigs on test service")
		}
		for _, ipconfig := range ipconfigs {
			props, err := ipconfig.GetIPProperties(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get properties of IPConfig")
			}
			if props.Address == intendedIP.String() {
				return errors.Errorf("%s is still in IPConfig", intendedIP.String())
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to wait for IPv4 address to disappear from IPConfigs: ", err)
	}

	// Verify the routing layer that IPv6 is still working while IPv4 should have
	// been stopped.
	if errs := testEnv.VerifyTestNetwork(ctx, routing.VerifyOptions{
		IPv4:      false,
		IPv6:      true,
		IsPrimary: true,
		Timeout:   5 * time.Second,
	}); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed to verify connectivity to the test network after DHCP failure: ", err)
		}
	}

	routerAddr, err := testEnv.TestRouter.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get addrs from test router: ", err)
	}
	serverAddr, err := testEnv.TestServer.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get addrs from test server: ", err)
	}
	for _, addr := range []string{routerAddr.IPv4Addr.String(), serverAddr.IPv4Addr.String()} {
		if err := ping.ExpectPingFailure(ctx, addr, "root"); err != nil {
			s.Errorf("%s should not be reachable: %v", addr, err)
		}
	}

	// Verify guest network.
	tc := s.Param().(lostDHCPTestCase)
	ipv6Addr := serverAddr.IPv6Addrs[0].String()
	switch tc {
	case loseDHCPTestCaseARC:
		a := s.FixtValue().(*arc.PreData).ARC
		vethName := testEnv.TestRouter.VethOutName
		arcIfname, err := arcnet.GetARCInterfaceName(ctx, vethName)
		if err != nil {
			s.Fatalf("Failed to get ARC interface name corresponding to %s: %v", vethName, err)
		}
		if err := arcnet.ExpectPingSuccess(ctx, a, arcIfname, ipv6Addr); err != nil {
			s.Errorf("Failed to verify IPv6 reachability to %s from ARC: %v", ipv6Addr, err)
		}
	case lostDHCPTestCaseCrostini:
		cont := s.FixtValue().(crostini.FixtureData).Cont
		if err := guestos.PingWithRetryAndTimeout(ctx, cont, ipv6Addr, 5*time.Second); err != nil {
			s.Errorf("Failed to verify IPv6 reachability to %s from Crostini: %v", ipv6Addr, err)
		}
	}
}
