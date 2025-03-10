// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type routingIPv4StaticTestCase struct {
	applyWhenConnecting bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     RoutingIPv4Static,
		Desc:     "Verify the shill behavior and routing semantics when the network does not have DHCP or SLAAC but only static IPv4 config",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn.ehide",
		// Using ehide in this test since there might be some problems with
		// StaticIPConfig due to the ethernet_any profile implementation
		// (b/159725895).
		Params: []testing.Param{{
			Name: "apply_when_idle",
			Val: routingIPv4StaticTestCase{
				applyWhenConnecting: false,
			},
		}, {
			Name: "apply_when_connecting",
			Val: routingIPv4StaticTestCase{
				applyWhenConnecting: true,
			},
		}},
	})
}

func RoutingIPv4Static(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	tc := s.Param().(routingIPv4StaticTestCase)
	disconnectBeforeApply := !tc.applyWhenConnecting

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// This test changes static IP configure. Ideally we want to call
	// LogOutUserAndPushTestProfile() here to avoid polluting the profile, but
	// it's not practical now since 1) test profile cannot be pushed on top of a
	// user profile and 2) we don't have a good way to push the test profile
	// before login with the chromeLoggedIn feature. If this becomes a problem,
	// we can consider doing StaticIPConfig cleanup in test hooks.

	testEnv := routing.NewTestEnv(cr)
	if err := testEnv.SetUpWithoutBaseNetwork(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	// Start a virtualnet with neither IPv4 nor IPv6, and configure static IP on
	// the router side.
	testNetworkOpts := virtualnet.EnvOptions{
		Priority:   routing.HighPriority,
		NameSuffix: routing.TestSuffix,
		EnableDHCP: false,
		RAServer:   false,
	}
	if err := testEnv.CreateNetworkEnvForTest(ctx, testNetworkOpts); err != nil {
		s.Fatal("Failed to create network for test: ", err)
	}

	// The allocated subnet has a /24 prefix.
	ipv4Subnet, err := testEnv.Pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv4 subnet for test network: ", err)
	}
	localIPv4Addr := ipv4Subnet.GetAddrEndWith(2)
	routerIPv4Addr := ipv4Subnet.GetAddrEndWith(1)
	if err := testEnv.TestRouter.ConfigureInterface(ctx, testEnv.TestRouter.VethInName, routerIPv4Addr, ipv4Subnet); err != nil {
		s.Fatal("Failed to configure IPv4 inside test router: ", err)
	}

	if disconnectBeforeApply {
		testing.ContextLog(ctx, "Disconnecting the test service")
		if err := testEnv.TestService.Disconnect(ctx); err != nil {
			s.Fatal("Failed to disconnect the test service: ", err)
		}
		if err := testEnv.TestService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateIdle, 5*time.Second); err != nil {
			s.Fatal("Failed to wait for the test service idle: ", err)
		}
	}

	// Configure static IP config on shill service.
	prefixLen := ipv4Subnet.PrefixLen()
	svcStaticIPConfig := map[string]interface{}{
		shillconst.IPConfigPropertyAddress:     localIPv4Addr.String(),
		shillconst.IPConfigPropertyGateway:     routerIPv4Addr.String(),
		shillconst.IPConfigPropertyPrefixlen:   prefixLen,
		shillconst.IPConfigPropertyNameServers: []string{routerIPv4Addr.String()},
	}
	testing.ContextLogf(ctx, "Configuring %v on the test interface", svcStaticIPConfig)
	if err := testEnv.TestService.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, svcStaticIPConfig); err != nil {
		s.Fatal("Failed to configure StaticIPConfig property on the test service: ", err)
	}
	defer func(ctx context.Context) {
		// Reset StaticIPConfig before removing the test interfaces, to avoid
		// installing this address on the physical interfaces. See
		// b/239753191#comment8 for a racing case.
		if err := testEnv.TestService.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, map[string]interface{}{}); err != nil {
			testing.ContextLog(ctx, "Failed to reset StaticIPConfig property on the test service: ", err)
		}
	}(cleanupCtx)

	if disconnectBeforeApply {
		testing.ContextLog(ctx, "Connect to the test service")
		if err := testEnv.TestService.Connect(ctx); err != nil {
			s.Fatal("Failed to connect the test service: ", err)
		}
	}

	testing.ContextLog(ctx, "Waiting for test service online")
	if err := testEnv.TestService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for the test service online: ", err)
	}

	testing.ContextLog(ctx, "Waiting for DHCP timeout event for ", routing.DHCPExtraTimeout)
	// GoBigSleepLint: Verify that the DHCP timeout event does not turn the
	// service down. We cannot trigger this event manually so nothing can be done
	// except for sleeping here.
	testing.Sleep(ctx, routing.DHCPExtraTimeout)
	testing.ContextLog(ctx, "DHCP timeout was triggered")

	// Verify the service state is still online.
	if err := testEnv.TestService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for the test service online after DHCP expired: ", err)
	}
	if state, err := testEnv.TestService.GetState(ctx); err != nil {
		s.Fatal("Failed to get state of the test service: ", err)
	} else if state != shillconst.ServiceStateOnline {
		s.Fatalf("Unexpected service state: got %s, want %s", state, shillconst.ServiceStateOnline)
	}

	// Verify routing setup for test network.
	if errs := testEnv.VerifyTestNetwork(ctx, routing.VerifyOptions{
		IPv4:      true,
		IPv6:      false,
		IsPrimary: true,
		Timeout:   5 * time.Second,
	}); len(errs) != 0 {
		for _, err := range errs {
			s.Error("Failed to verify test network after configuring static IP: ", err)
		}
	}

	// Verify IPConfigs.
	ipconfigs, err := testEnv.TestService.GetIPConfigs(ctx)
	if err != nil {
		s.Fatal("Failed to get IPConfigs: ", err)
	}
	if len(ipconfigs) != 1 {
		s.Fatal("Expect 1 IPConfig objects, but got ", len(ipconfigs))
	}
	actualIPProps, err := ipconfigs[0].GetIPProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get IPProperties from IPConfig: ", err)
	}
	expectedIPProps := shill.IPProperties{
		Address:     localIPv4Addr.String(),
		Gateway:     routerIPv4Addr.String(),
		Method:      "ipv4",
		PrefixLen:   int32(prefixLen),
		NameServers: []string{routerIPv4Addr.String()},
	}
	if diff := cmp.Diff(actualIPProps, expectedIPProps); diff != "" {
		s.Fatal("Got unexpected IPProperties with diff: ", diff)
	}
}
