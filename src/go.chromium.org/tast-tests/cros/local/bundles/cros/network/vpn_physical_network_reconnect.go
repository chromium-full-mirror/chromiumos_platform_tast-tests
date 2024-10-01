// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
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
	expectVPNDownOnPhysicalDown bool
	// Restart server to apply new config after underlying network is down, to
	// verify that the new config takes effect after VPN is reconnected. This
	// option is only meaningful is the configuration is pushed from the server.
	pushNewConfigAfterPhysicalDown bool
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
				expectVPNDownOnPhysicalDown: true,
				vpnDownTimeout:              40 * time.Second,
			},
			Fixture:           "shillSimulatedWiFiWithCerts.ehide",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "openvpn_new_config",
			Val: vpnReconnectParams{
				vpnType:                        vpn.TypeOpenVPN,
				expectVPNDownOnPhysicalDown:    true,
				pushNewConfigAfterPhysicalDown: true,
				vpnDownTimeout:                 40 * time.Second,
			},
			Fixture:           "shillSimulatedWiFiWithCerts.ehide",
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}, {
			Name: "wireguard",
			Val: vpnReconnectParams{
				vpnType:                     vpn.TypeWireGuard,
				expectVPNDownOnPhysicalDown: false,
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

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDisablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

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

	param := s.Param().(vpnReconnectParams)
	certVals := s.FixtValue().(*hwsim.ShillSimulatedWiFi).CertVals

	// Create VPN service and server. Do not use StartConnection() here since we
	// may start a different server later.
	config := vpn.NewConfig(
		param.vpnType,
		vpn.WithCertVals(certVals),
		vpn.WithIPType(vpn.IPTypeIPv4),
	)
	server, err := vpn.StartServerWithConfig(ctx, serverEnv, config)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer func() {
		if server == nil {
			return
		}
		if err := server.StopServer(cleanupCtx); err != nil {
			s.Fatal("Failed to stop VPN server: ", err)
		}
	}()
	service, err := vpn.ConfigureService(ctx, server, nil /*secondServer*/)
	if err != nil {
		s.Fatal("Failed to configure VPN service: ", err)
	}
	defer func() {
		if err := service.Remove(cleanupCtx); err != nil {
			s.Fatal("Failed to remove VPN service: ", err)
		}
	}()

	// Connect the service and verify the connection.
	if err := service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to VPN service: ", err)
	}
	if err := service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for connected state: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %s", server.OverlayIPv4, err)
	}

	s.Log("Disconnecting physical network")
	if err := wifi.Service.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect WiFi: ", err)
	}

	s.Log("Verifying VPN is disconnected after physical network disconnected")
	// Check VPN reports to be in a disconnected state if supported.
	if param.expectVPNDownOnPhysicalDown {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			connected, err := service.IsConnected(ctx)
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
		if err := ping.ExpectPingFailure(ctx, server.OverlayIPv4, "chronos"); err != nil {
			s.Fatal("Expected ping fail after disconnect: ", err)
		}
	}

	s.Log("Verified VPN is disconnected")

	if param.pushNewConfigAfterPhysicalDown {
		newIPSubnet, err := pool.AllocNextIPv4Subnet()
		if err != nil {
			s.Fatal("Failed to allocate IPv4 subnet for new VPN config: ", err)
		}
		newConfig := vpn.NewConfig(
			param.vpnType,
			vpn.WithCertVals(certVals),
			vpn.WithIPv4Subnet(newIPSubnet),
		)
		if err := server.StopServer(ctx); err != nil {
			s.Fatal("Failed to stop VPN server: ", err)
		}
		server, err = vpn.StartServerWithConfig(ctx, serverEnv, newConfig)
		if err != nil {
			s.Fatal("Failed to restart VPN server with new config: ", err)
		}
		// Defer cleanup for server is set up above.
	}

	s.Log("Reconnecting physical network")
	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to reconnect WiFi: ", err)
	}
	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for to WiFi reconnected status: ", err)
	}

	// Verify VPN reconnection.
	if err := service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for connected state after reconnection: ", err)
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, server.OverlayIPv4, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s after reconnection: %s", server.OverlayIPv4, err)
	}
}
