// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	tdreq "chromiumos/tast/common/testdevicerequirements"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: P2PConcurrencyFunc,
		Desc: "Tests the concurrent functionality of both WiDi and Infra WiFi",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_p2p", "wificell_cross_device_unstable"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      "wificellFixtCompanionDut",
		Requirements: []string{tdreq.WiFiGenSupportWFD},
	})
}

func P2PConcurrencyFunc(ctx context.Context, s *testing.State) {
	/*
		This test aims to test the concurrent functionality of both WiDi and Infra WiFi by using
		the following steps:
		1- Configure and connect the p2p link (main DUT: GO, Companion DUT: Client)
		2- Verify the p2p connection (main DUT --> Companion DUT).
		3- Configure and connect the Infra WiFi link (main DUT: Client, Router)
		4- Verify p2p and Infra WiFi connections:
		4- 1- The p2p connection (main DUT --> Companion DUT).
		4- 2- The Infra WiFi connection (main DUT --> Router).
		5- Deconfigure the p2p link.
		6- Verify the Infra WiFi connection (main DUT --> Router).
		7- Configure and connect the p2p link (main DUT: GO, Companion DUT: Client)
		8- Verify Infra Wifi and p2p connections:
		8- 1- The Infra WiFi connection (main DUT --> Router).
		8- 2- The p2p connection (main DUT --> Companion DUT).
		9- Deconfigure the Infra WiFi link.
		10- Verify the p2p connection (main DUT --> Companion DUT).
		11- Deconfigure the p2p link.
	*/

	tf := s.FixtValue().(*wificell.TestFixture)

	P2PGOIsConfigured := false
	P2PClientIsConfigured := false
	P2PIPRouteIsConfigured := false
	configureP2PConnection := func(ctx context.Context) {
		successfulRun := false
		if err := tf.P2PConfigureGO(ctx, wificell.P2PDeviceDUT); err != nil {
			s.Fatal("Failed to configure the p2p group owner (GO): ", err)
		}
		P2PGOIsConfigured = true
		defer func(ctx context.Context) {
			if P2PGOIsConfigured && !successfulRun {
				if err := tf.P2PDeconfigureGO(ctx); err != nil {
					s.Error("Failed to deconfigure the p2p group owner (GO): ", err)
				}
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigP2P(ctx)
		defer cancel()
		if err := tf.P2PConfigureClient(ctx, wificell.P2PDeviceCompanionDUT); err != nil {
			s.Fatal("Failed to configure the p2p client: ", err)
		}
		P2PClientIsConfigured = true
		defer func(ctx context.Context) {
			if P2PClientIsConfigured && !successfulRun {
				if err := tf.P2PDeconfigureClient(ctx); err != nil {
					s.Error("Failed to deconfigure the p2p client: ", err)
				}
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDeconfigP2P(ctx)
		defer cancel()
		if err := tf.P2PConnect(ctx); err != nil {
			s.Fatal("Failed to connect the p2p client to the p2p group owner (GO) network: ", err)
		}
		if err := tf.P2PAddIPRoute(ctx); err != nil {
			s.Fatal("Failed to route the IP addresses in the p2p group owner (GO) and the p2p client: ", err)
		}
		P2PIPRouteIsConfigured = true
		defer func(ctx context.Context) {
			if P2PIPRouteIsConfigured && !successfulRun {
				if err := tf.P2PDeleteIPRoute(ctx); err != nil {
					s.Error("Failed to delete the IP routing in the p2p group owner and p2p client: ", err)
				}
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDeleteIPRoute(ctx)
		defer cancel()
		successfulRun = true
	}

	deconfigureP2PConnection := func(ctx context.Context) {
		s.Log("Deconfigure P2P connection")
		if P2PIPRouteIsConfigured {
			if err := tf.P2PDeleteIPRoute(ctx); err != nil {
				s.Fatal("Failed to delete the IP routing in the p2p group owner and p2p client: ", err)
			}
			P2PIPRouteIsConfigured = false
		}
		if P2PClientIsConfigured {
			if err := tf.P2PDeconfigureClient(ctx); err != nil {
				s.Fatal("Failed to deconfigure the p2p client: ", err)
			}
			P2PClientIsConfigured = false
		}

		if P2PGOIsConfigured {
			if err := tf.P2PDeconfigureGO(ctx); err != nil {
				s.Fatal("Failed to deconfigure the p2p group owner (GO): ", err)
			}
			P2PGOIsConfigured = false
		}
	}

	APIsConfigured := false
	APIsConnected := false
	var currAP *wificell.APIface
	configureInfraWiFiConnection := func(ctx context.Context) {
		successfulRun := true
		ap, err := tf.DefaultOpenNetworkAP(ctx)
		if err != nil {
			s.Fatal("Failed to configure AP: ", err)
		}
		currAP = ap
		APIsConfigured = true
		defer func(ctx context.Context) {
			if APIsConfigured && !successfulRun {
				if err := tf.DeconfigAP(ctx, ap); err != nil {
					s.Error("Failed to deconfig AP: ", err)
				}
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
			s.Fatal("Failed to connect to WiFi: ", err)
		}
		APIsConnected = true
		defer func(ctx context.Context) {
			if APIsConnected && !successfulRun {
				if err := tf.CleanDisconnectWifi(ctx); err != nil {
					s.Error("Failed to disconnect WiFi: ", err)
				}
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		successfulRun = true
	}

	deconfigureInfraWiFiConnection := func(ctx context.Context) {
		if APIsConnected {
			s.Log("Deconfigure the Infra WiFi connection")
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
			APIsConnected = false
		}
		if APIsConfigured {
			if err := tf.DeconfigAP(ctx, currAP); err != nil {
				s.Error("Failed to deconfig AP: ", err)
			}
			APIsConfigured = false
		}
	}

	verifyP2PConnection := func(ctx context.Context) {
		s.Log("Verifying the P2P connection")
		if err := tf.P2PAssertPingFromGO(ctx); err != nil {
			s.Fatal("Failed to ping the p2p client from the p2p group owner (GO): ", err)
		}
		if err := tf.P2PAssertPingFromClient(ctx); err != nil {
			s.Fatal("Failed to ping p2p group onwer (GO) from the p2p client: ", err)
		}
	}

	verifyInfraWiFiConnection := func(ctx context.Context) {
		s.Log("Verifying the Infra WiFi connection")
		if err := tf.VerifyConnection(ctx, currAP); err != nil {
			s.Fatal("Failed to verify connection: ", err)
		}
	}

	// Create the P2P connection.
	configureP2PConnection(ctx)
	defer deconfigureP2PConnection(ctx)

	// Verify the P2P connection.
	verifyP2PConnection(ctx)

	// Create the Infra WiFi connection.
	configureInfraWiFiConnection(ctx)
	defer deconfigureInfraWiFiConnection(ctx)

	// Verify both P2P and Infra WiFi connections.
	verifyP2PConnection(ctx)
	verifyInfraWiFiConnection(ctx)

	// Deconfigure the P2P connection.
	deconfigureP2PConnection(ctx)

	// Verify the Infra WiFi connection.
	verifyInfraWiFiConnection(ctx)

	// Configure the P2P connection again.
	configureP2PConnection(ctx)
	defer deconfigureP2PConnection(ctx)

	// Verify both the P2P and Infra WiFi connections.
	verifyP2PConnection(ctx)
	verifyInfraWiFiConnection(ctx)

	// Deconfigure the Infra WiFi connection.
	deconfigureInfraWiFiConnection(ctx)

	// Verify the P2P connection.
	verifyP2PConnection(ctx)
}
