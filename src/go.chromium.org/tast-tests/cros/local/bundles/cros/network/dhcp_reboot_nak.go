// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPRebootNAK,
		Desc:     "Verify the DHCP behavior in the case that DHCP server rejects the rebinding request with the previous lease",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "shillSimulatedWiFi",
	})
}

// DHCPRebootNAK verifies the DHCP behavior in the following scenario:
//
//  1. DUT (DHCP client) does the DHCP negotiation with the server, gets a lease,
//     and enters the BOUND state.
//  2. Disconnect the network and connect again. Since there is a valid lease,
//     DUT sends a DHCPREQUEST to rebind to this lease and enters the REBOOTING
//     state.
//  3. DHCP server rejects this request by sending back a NAK packet.
//  4. DUT enters the INIT state and restart the DHCP negotiation.
//  5. DHCP server offers a lease with another address this time.
func DHCPRebootNAK(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	popFunc, err := shill.LogOutUserAndPushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to push test profile: ", err)
	}
	defer popFunc(cleanupCtx)

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Prepare the environment.
	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], manager, pool, virtualnet.EnvOptions{})
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	// Disable MAR to make sure the reconnect reuse the lease.
	if err := wifi.Service.SetProperty(ctx, shillconst.ServicePropertyWiFiRandomMACPolicy, shillconst.MacPolicyHardware); err != nil {
		s.Fatal("Failed to disable MAR for the WiFi service: ", err)
	}

	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	gatewayIP := subnet.GetAddrEndWith(1)
	intendedIP := subnet.GetAddrEndWith(2)
	intendedIPAfterNAK := subnet.GetAddrEndWith(3)

	// Install gateway address and routes.
	if err := wifi.Router.ConfigureInterface(ctx, wifi.Router.VethInName, gatewayIP, subnet); err != nil {
		s.Fatal("Failed to install address on router: ", err)
	}

	// Connect the service, and verify that:
	// - DHCP server receives the DISCOVER and REQUEST packets in order.
	// - The service state becomes connected.
	testing.ContextLog(ctx, "Verifying DHCP negotiation")

	dhcpOpts := dhcp.NewOptionMap(gatewayIP, intendedIP)
	discoverRule := dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/)
	requestRule := dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, true, /*shouldRespond*/
		gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/)
	requestRule.SetIsFinalHandler(true)

	if _, errs := dhcp.RunTestWithEnv(ctx, wifi.Router, []dhcp.HandlingRule{*discoverRule, *requestRule}, func(ctx context.Context) error {
		if err := wifi.Service.Connect(ctx); err != nil {
			return err
		}
		return wifi.Service.WaitForConnectedOrError(ctx)
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
		return
	}

	// Disconnect and connect the service, verify that:
	// - DHCP server receives the REQUEST, DISCOVER, and then another REQUEST in order.
	// - Shill service state becomes connected and it takes the new lease (address).
	testing.ContextLog(ctx, "Verifying DHCP rebinding")

	// In this test, dhcpcd will defer the process for NAK for 4 seconds. Thus we
	// need a longer poll timeout here.
	const rebindingTimeout = 10 * time.Second

	nakRule := dhcp.NewRejectRequestRule(intendedIP.String())
	dhcpOptsAfterNAK := dhcp.NewOptionMap(gatewayIP, intendedIPAfterNAK)
	discoverRuleAfterNAK := dhcp.NewRespondToDiscovery(intendedIPAfterNAK.String(), gatewayIP.String(),
		dhcpOptsAfterNAK, dhcp.FieldMap{}, true /*shouldRespond*/)
	requestRuleAfterNAK := dhcp.NewRespondToRequest(intendedIPAfterNAK.String(), gatewayIP.String(),
		dhcpOptsAfterNAK, dhcp.FieldMap{}, true, /*shouldRespond*/
		gatewayIP.String(), intendedIPAfterNAK.String(), true /*expSvrIPSet*/)
	requestRuleAfterNAK.SetIsFinalHandler(true)

	rules := []dhcp.HandlingRule{*nakRule, *discoverRuleAfterNAK, *requestRuleAfterNAK}
	if _, errs := dhcp.RunTestWithEnv(ctx, wifi.Router, rules, func(ctx context.Context) error {
		if err := wifi.Service.Reconnect(ctx); err != nil {
			return errors.Wrap(err, "failed to reconnect the WiFi service")
		}

		return testing.Poll(ctx, func(ctx context.Context) error {
			ipconfigs, err := wifi.Service.GetIPConfigs(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get IPConfigs on service")
			}
			if len(ipconfigs) != 1 {
				return errors.Errorf("len(ipconfigs)=%d, want %d", len(ipconfigs), 1)
			}
			props, err := ipconfigs[0].GetIPProperties(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get IPProperties from the IPConfig object")
			}
			if props.Address != intendedIPAfterNAK.String() {
				return errors.Errorf("unexpected address: got %s, want %s", props.Address, intendedIPAfterNAK.String())
			}
			return nil
		}, &testing.PollOptions{Timeout: rebindingTimeout})
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation for reconnect: ", err)
		}
	}
}
