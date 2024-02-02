// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type alwaysOnVPNRoutingTestCase struct {
	mode    string // mode of always-on VPN we want to test.
	vpnType vpn.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AlwaysOnVPNRouting,
		Desc:         "Host VPN client can be configured as always-on VPN and connected automatically",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "vpnEnv",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "strict_mode_ikev2",
				Val: alwaysOnVPNRoutingTestCase{
					mode:    shillconst.AlwaysOnVPNModeStrict,
					vpnType: vpn.TypeIKEv2,
				},
				ExtraSoftwareDeps: []string{"ikev2"},
			},
			{
				Name: "strict_mode_l2tp_ipsec_psk",
				Val: alwaysOnVPNRoutingTestCase{
					mode:    shillconst.AlwaysOnVPNModeStrict,
					vpnType: vpn.TypeL2TPIPsec,
				},
			},
			{
				Name: "best_effort_mode_ikev2",
				Val: alwaysOnVPNRoutingTestCase{
					mode:    shillconst.AlwaysOnVPNModeBestEffort,
					vpnType: vpn.TypeIKEv2,
				},
				ExtraSoftwareDeps: []string{"ikev2"},
			},
			{
				Name: "best_effort_mode_l2tp_ipsec_psk",
				Val: alwaysOnVPNRoutingTestCase{
					mode:    shillconst.AlwaysOnVPNModeBestEffort,
					vpnType: vpn.TypeL2TPIPsec,
				},
			},
		},
	})
}

// AlwaysOnVPNRouting sets up an host VPN and checks if this VPN can be configured
// as always-on VPN, connects automatically and routing is correct when VPN service
// is not available.
//
// In this test, we set up the network as follows (by vpn.CreateNetworkTopology):
//
//	veth0 --+-- test router --+-- physical server (used to test if system/user traffic is blocked)
//	            DHCP server   |
//	                          +-- virtual server (used to configure Always-on VPN)
func AlwaysOnVPNRouting(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	// Set up an test profile and pop it out on stack after test is finished.
	popFunc, err := shill.LogOutUserAndPushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to push test profile: ", err)
	}
	defer popFunc(cleanupCtx)

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

	addrs, err := physicalEnv.WaitForVethInAddrs(ctx, true, false)
	if err != nil {
		s.Fatal("Failed to get server addrs: ", err)
	}
	physicalAddr := addrs.IPv4Addr.String()

	// Establish a VPN connection on one of the servers.
	conn, err := vpn.StartConnection(ctx, vpnEnv,
		s.Param().(alwaysOnVPNRoutingTestCase).vpnType,
		vpn.WithoutAutoConnect(),
	)
	if err != nil {
		s.Fatal("Failed to create VPN connection: ", err)
	}
	defer conn.Cleanup(cleanupCtx)

	// Use set up host VPN as service and change the Always-on VPN mode.
	vpnMode := s.Param().(alwaysOnVPNRoutingTestCase).mode
	s.Logf("Setting always-on-vpn mode in shill to %s and waiting for VPN connected", vpnMode)
	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		s.Fatal("Failed to get active profile: ", err)
	}
	// We use a test profile here so don't need to reset the value.
	if err := profile.SetAlwaysOnVPN(ctx, vpnMode, conn.Service()); err != nil {
		s.Fatal("Failed to set Always-on VPN properties: ", err)
	}

	// Check if always on VPN is set in correct mode.
	props, err := profile.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get props: ", err)
	}
	if curMode, err := props.GetString(shillconst.ProfilePropertyAlwaysOnVPNMode); err != nil {
		s.Fatal("Failed to get Always-on VPN mode: ", err)
	} else if curMode != vpnMode {
		s.Errorf("Current Always-on VPN mode is %v, want: %v", curMode, vpnMode)
	}

	// Check if VPN can be automatically connected.
	if err := conn.Service().WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for VPN connected automatically: ", err)
	}

	// Disconnect VPN and block all traffic in VPN netNS, make sure VPN does not re-connect.
	s.Log("VPN has been connected. Disconnect and block the VPN connection and then verify the routing")
	if err := conn.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect from VPN: ", err)
	}

	cmd := []string{"iptables", "-w", "-I", "INPUT", "-j", "DROP"}
	if err := vpnEnv.RunWithoutChroot(ctx, cmd...); err != nil {
		s.Fatal("Failed to block all traffic for VPN: ", err)
	}

	// Test system traffic is not blocked if the VPN is not connectable for both modes.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, physicalAddr, "root", 10*time.Second); err != nil {
		s.Errorf("User %s failed to ping %v: %v", "root", physicalAddr, err)
	}

	// Test user traffic is blocked by strict mode but not best effort mode.
	const user = "chronos"
	switch vpnMode {
	case shillconst.AlwaysOnVPNModeStrict:
		if err := ping.ExpectPingFailure(ctx, physicalAddr, user); err != nil {
			s.Errorf("User %s succeeded to ping %v in strict mode: %v", user, physicalAddr, err)
		}
	case shillconst.AlwaysOnVPNModeBestEffort:
		if err := ping.ExpectPingSuccessWithTimeout(ctx, physicalAddr, user, 10*time.Second); err != nil {
			s.Errorf("User %s failed to ping %v: %v", user, physicalAddr, err)
		}
	}
}
