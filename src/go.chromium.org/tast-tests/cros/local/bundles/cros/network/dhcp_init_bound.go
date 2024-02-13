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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPInitBound,
		Desc:     "Verifies DHCP negotiation behavior on a WiFi network",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "shillSimulatedWiFi",
	})
}

// DHCPInitBound verifies the DHCP negotiation behavior on a WiFi network
// without MAC randomization:
//   - When connecting to the AP for the first time, the DHCP client should
//     enter the INIT state, a DISCOVER and then a REQUEST packet will be sent
//     for the negotiation.
//   - When connecting to same AP again and we still hold a valid lease, the
//     DHCP client should enter the INIT-REBOOT state, only one REQUEST packet
//     will be sent for the negotiation.
func DHCPInitBound(ctx context.Context, s *testing.State) {
	//Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], m, pool, virtualnet.EnvOptions{})
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	// Disable MAR.
	if err := wifi.Service.SetProperty(ctx, shillconst.ServicePropertyWiFiRandomMACPolicy, shillconst.MacPolicyHardware); err != nil {
		s.Fatal("Failed to disable MAR for the WiFi service: ", err)
	}

	// Create subnet for DHCP.
	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	gatewayIP := subnet.GetAddrEndWith(1)
	intendedIP := subnet.GetAddrEndWith(2)

	// Install gateway address and routes.
	if err := wifi.Router.ConfigureInterface(ctx, wifi.Router.VethInName, gatewayIP, subnet); err != nil {
		s.Fatal("Failed to install address on router: ", err)
	}

	discoveryRule := dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
		dhcp.NewOptionMap(gatewayIP, intendedIP), dhcp.FieldMap{}, true /*shouldRespond*/)
	requestRule := dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
		dhcp.NewOptionMap(gatewayIP, intendedIP), dhcp.FieldMap{}, true, /*shouldRespond*/
		gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/)
	requestRule.SetIsFinalHandler(true)

	// Connect the service for the first time. Configure the server with DISCOVER
	// and REQUEST rules.
	if _, errs := dhcp.RunTestWithEnv(ctx, wifi.Router, []dhcp.HandlingRule{*discoveryRule, *requestRule}, func(ctx context.Context) error {
		if err := wifi.Service.Connect(ctx); err != nil {
			return err
		}
		if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
			return err
		}
		return nil
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
		return
	}

	if err := wifi.Service.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect from the WiFi service: ", err)
	}

	// Reconnect the service. Configure the server with REQUEST rule. Note that
	// server id must not be included in the request. See RFC 2131 for more
	// details.
	requestRule = dhcp.NewRespondToPostT2Request(intendedIP.String(), gatewayIP.String(),
		dhcp.NewOptionMap(gatewayIP, intendedIP), dhcp.FieldMap{}, true /*shouldRespond*/, intendedIP.String())
	requestRule.SetIsFinalHandler(true)
	if _, errs := dhcp.RunTestWithEnv(ctx, wifi.Router, []dhcp.HandlingRule{*requestRule}, func(ctx context.Context) error {
		if err := wifi.Service.Connect(ctx); err != nil {
			return err
		}
		if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
			return err
		}
		return nil
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation for reconnect: ", err)
		}
	}
}
