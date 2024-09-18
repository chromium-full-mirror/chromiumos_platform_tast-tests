// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type vpnReconnectParams struct {
	vpnType vpn.Type
	// If the VPN type reports disconnect when underlying network is down.
	expectVpnDownOnPhysicalDown bool
	// Timeout before VPN reports disconnect when underlying network is down.
	vpnDownTimeout time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNPhysicalNetworkReconnect,
		Desc:     "Checks VPN behavior on physical network disconnected and reconnected",
		Contacts: []string{"cros-networking@google.com", "ningyuan@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		LacrosStatus: testing.LacrosVariantUnneeded,
		// TODO(258091734): Move to CQ after test is stable.
		Attr: []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Name: "openvpn",
			Val: vpnReconnectParams{
				vpnType:                     vpn.TypeOpenVPN,
				expectVpnDownOnPhysicalDown: true,
				vpnDownTimeout:              40 * time.Second,
			},
			Fixture:           "shillSimulatedWiFiWithCerts.ehide",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "wireguard",
			Val: vpnReconnectParams{
				vpnType:                     vpn.TypeWireGuard,
				expectVpnDownOnPhysicalDown: false,
			},
			Fixture:           "shillSimulatedWiFi.ehide",
			ExtraSoftwareDeps: []string{"wireguard"},
		}},
	})
}

func VPNPhysicalNetworkReconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 30*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	apInterface := s.FixtValue().(*hwsim.ShillSimulatedWiFi).AP[0]
	pool := subnet.NewPool()
	routerOpts := virtualnet.EnvOptions{EnableDHCP: true}
	// Create wifi router env and connect to it.
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, apInterface, m, pool, routerOpts)
	if err != nil {
		s.Fatal("Failed to create WiFi env: ", err)
	}
	defer func(ctx context.Context) {
		if err := wifi.Cleanup(ctx); err != nil {
			s.Error("Failed to clean up virtual WiFi env: ", err)
		}
	}(cleanupCtx)

	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for to WiFi connected status: ", err)
	}

	// Create VPN server env.
	serverEnv, err := virtualnet.CreateEnv(ctx, "server")
	if err != nil {
		s.Fatal("Failed to create server env: ", err)
	}
	defer func(ctx context.Context) {
		if err := serverEnv.Cleanup(ctx); err != nil {
			s.Error("Failed to clean up server env: ", err)
		}
	}(cleanupCtx)
	if err := serverEnv.ConnectToRouterWithPool(ctx, wifi.Router, pool); err != nil {
		s.Fatal("Failed to connect server env to WiFi env: ", err)
	}

	// Start VPN connection.
	param := s.Param().(vpnReconnectParams)
	vpnConn, err := vpn.StartConnection(ctx, serverEnv, param.vpnType, vpn.WithCertVals(s.FixtValue().(*hwsim.ShillSimulatedWiFi).CertVals))
	if err != nil {
		s.Fatal("Failed to start VPN connection: ", err)
	}
	defer func() {
		if err := vpnConn.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN connection: ", err)
		}
	}()

	if err := wifi.Service.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect WiFi: ", err)
	}
	s.Log("Verifying VPN is disconnected after physical network disconnected")
	// Check VPN reports to be in a disconnected state if supported.
	if param.expectVpnDownOnPhysicalDown {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			connected, err := vpnConn.Service().IsConnected(ctx)
			if err != nil {
				testing.PollBreak(err)
			} else if connected {
				return errors.New("VPN still report as connected")
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  param.vpnDownTimeout,
			Interval: 1 * time.Second,
		}); err != nil {
			s.Fatal("Failed to wait for VPN service to be not connected: ", err)
		}
	} else {
		if err := ping.ExpectPingFailure(ctx, vpnConn.Server.OverlayIPv4, "chronos"); err != nil {
			s.Fatal("Expected ping fail after disconnect: ", err)
		}
	}
	s.Log("Verified VPN is disconnected. Reconnecting physical network")

	// Reconnect WiFi.
	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to reconnect WiFi: ", err)
	}
	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for to WiFi reconnected status: ", err)
	}

	// Verify VPN reconnection:
	if err := vpnConn.Service().WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for connected state: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, vpnConn.Server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %s", vpnConn.Server.OverlayIPv4, err)
	}
}
