// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type p2pConcurrencyRoamTestcase struct {
	ap2Config hostapd.ApConfig
	ap1Config hostapd.ApConfig
	p2pOpts   []p2p.GroupOption
}

func init() {
	testing.AddTest(&testing.Test{
		Func: P2PConcurrencyRoam,
		Desc: "Tests P2P cuncurrency during Infra-WiFi roaming",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_p2p", "wificell_cross_device_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		HardwareDepsForAll: map[string]hwdep.Deps{
			"":    hwdep.D(hwdep.WifiP2P()),
			"cd1": hwdep.D(hwdep.WifiP2P()),
		},
		Requirements: []string{tdreq.WiFiGenSupportWFD},
		Params: []testing.Param{
			{
				Name: "chromebook_chromebook_same_chan_2g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(11), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_same_chan_5g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(36), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(11), hostapd.HTCaps(hostapd.HTCapHT20)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_diff_chan_2g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(36), hostapd.HTCaps(hostapd.HTCapHT20)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_diff_chan_5g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(11), hostapd.HTCaps(hostapd.HTCapHT20)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}},
	})
}

func P2PConcurrencyRoam(ctx context.Context, s *testing.State) {
	/*
		This test aims to test the concurrent functionality of both WiFi Direct and Infra WiFi. The test will cover the following scenario:
		1-	Configure Infra WiFi links (DUT → Router, Companion DUT → Router).
		2-	Verify the connection for all links using ICMP ping.
		3-	Configure a P2P link: DUT (GO) <-> Companion DUT (Client).
		4-	Verify the connection for all links using ICMP ping (P2P + Infra WiFi).
		5-	Configure 2nd Infra BSSID, based on test variant: on the same channel, different channel or different band.
		6-	Trigger roaming on P2P GO (BSSTM).
		7-	Verify the connection for all links using ICMP ping (Infra WiFi + P2P).
		8-	Trigger roaming on P2P Client (BSSTM).
		9-	Verify the connection for all links using ICMP ping (Infra WiFi + P2P).
		10-	Deconfigure the P2P link.
		11-	Disconnect Infra links.

	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(p2pConcurrencyRoamTestcase)

	ctx, rt, finish, err := wifiutil.SimpleRoamInitialSetup(ctx, tf, []wificell.DutIdx{wificell.DefaultDUT, wificell.PeerDUT1}, tc.ap1Config, tc.ap2Config, false)
	if err != nil {
		s.Fatal("Failed initial setup of the test: ", err)
	}
	defer func() {
		if err := finish(); err != nil {
			s.Error("Error while tearing down test setup: ", err)
		}
	}()
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := tf.P2PConfigureGO(ctx, wificell.P2PDevice(wificell.DefaultDUT), tc.p2pOpts...); err != nil {
		s.Fatal("Failed to configure the p2p group owner (GO): ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.P2PDeconfigureGO(ctx); err != nil {
			s.Error("Failed to deconfigure the p2p group owner (GO): ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigP2P(ctx)
	defer cancel()

	if err := tf.P2PConnect(ctx, wificell.P2PDevice(wificell.PeerDUT1)); err != nil {
		s.Fatal("Failed to connect the p2p client to the p2p group owner (GO) network: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.P2PDisconnect(ctx); err != nil {
			s.Error("Failed to deconfigure the p2p client: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigP2P(ctx)
	defer cancel()

	if err := tf.P2PAssertPingFromGO(ctx); err != nil {
		s.Fatal("Failed to ping the p2p client from the p2p group owner (GO): ", err)
	}
	if err := tf.P2PAssertPingFromClient(ctx); err != nil {
		s.Fatal("Failed to ping p2p group owner (GO) from the p2p client: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, rt.AP1()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.PeerDUT1, rt.AP1()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}

	fromBSSID := rt.AP1BSSID()
	roamBSSID := rt.AP2BSSID()
	testSSID := rt.AP1SSID()
	s.Log("AP 1 BSSID: ", fromBSSID)
	s.Log("AP 2 BSSID: ", roamBSSID)

	sendReqAndWaitConnected := func(dut wificell.DutIdx, from, to string, fromAP, toAP *wificell.APIface, req hostapd.BSSTMReqParams, servicePath string) {
		// Get the name and MAC address of the DUT WiFi interface.
		dutIface, err := tf.DUTClientInterface(ctx, dut)
		if err != nil {
			s.Fatal("Unable to get DUT interface name: ", err)
		}
		dutMACAddr, err := tf.DUTHardwareAddr(ctx, dut)
		if err != nil {
			s.Fatal("Unable to get DUT MAC address: ", err)
		}
		dutMAC := dutMACAddr.String()
		// Flush all scanned BSS from wpa_supplicant so that test behavior is consistent.
		s.Log("Flushing BSS cache")
		if err := tf.DUTWifiClient(dut).FlushBSS(ctx, dutIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}
		s.Logf("Waiting for roamBSSID: %s", roamBSSID)
		if err := tf.DUTWifiClient(dut).DiscoverBSSID(ctx, roamBSSID, dutIface, []byte(testSSID)); err != nil {
			s.Fatal("Unable to discover roam BSSID: ", err)
		}
		err = tf.ClearBSSIDIgnoreDUT(ctx, dut)
		if err != nil {
			s.Fatal("Failed to clear wpa BSSID_IGNORE: ", err)
		}
		// Before sending the BSSTM request, add the current BSSID into the
		// DUT's ignorelist to avoid any any potential race condition. Adding a
		// BSSID to the ignore list does not trigger the device to roam away
		// from the BSSID, but it should prevent it from roaming back.
		// NB: Each time we add the BSSID to the ignore list, it increases
		// the duration for which the BSSID is ignored. Each wpa_cli
		// invocation results in the BSSID being added to the ignore list
		// twice, so the two calls here translate to 4 insertions in
		// wpa_supplicant, which results in an ignorelist duration of 120
		// seconds, which should be plenty.

		// We add the BSSID into the ignorelist twice purposely to ensure the
		// ignore duration is sufficient.
		err = tf.AddToBSSIDIgnoreDUT(ctx, dut, fromBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}
		err = tf.AddToBSSIDIgnoreDUT(ctx, dut, fromBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}

		// Set up a watcher for the Shill WiFi BSSID property.
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		waitForProps, err := tf.DUTWifiClient(dut).GenerateRoamPropertyWatcher(waitCtx, to, servicePath)
		// Send BSS Transition Management Request to client.
		s.Logf("Sending BSS Transition Management Request from AP %s to DUT %s", from, dutMAC)
		if err := fromAP.SendBSSTMRequest(ctx, dutMAC, req); err != nil {
			s.Fatal("Failed to send BSS TM Request: ", err)
		}

		// Wait for the DUT to roam to the second AP, then assert that there was
		// no disconnection during roaming.
		s.Log("Waiting for roaming")
		monitorResult, err := waitForProps()
		if err != nil {
			s.Fatal("Failed to roam within timeout: ", err)
		}

		if err := wifiutil.VerifyNoDisconnections(monitorResult); err != nil {
			s.Fatal("DUT: failed to stay connected during the roaming process: ", err)
		}
		// Just for good measure make sure we're properly connected.
		s.Log("Verifying connection to AP ", to)
		if err := tf.VerifyConnection(ctx, toAP); err != nil {
			s.Fatal("DUT: failed to verify connection: ", err)
		}
	}

	var requestParams hostapd.BSSTMReqParams
	requestParams.Neighbors = []string{roamBSSID}
	sendReqAndWaitConnected(wificell.DefaultDUT, fromBSSID, roamBSSID, rt.AP1(), rt.AP2(), requestParams, rt.ServicePath())
	if err := tf.P2PAssertPingFromGO(ctx); err != nil {
		s.Fatal("Failed to ping the p2p client from the p2p group owner (GO): ", err)
	}
	if err := tf.P2PAssertPingFromClient(ctx); err != nil {
		s.Fatal("Failed to ping p2p group owner (GO) from the p2p client: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, rt.AP2()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.PeerDUT1, rt.AP1()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}

	sendReqAndWaitConnected(wificell.PeerDUT1, fromBSSID, roamBSSID, rt.AP1(), rt.AP2(), requestParams, rt.ServicePathOfDUT(wificell.PeerDUT1))
	if err := tf.P2PAssertPingFromGO(ctx); err != nil {
		s.Fatal("Failed to ping the p2p client from the p2p group owner (GO): ", err)
	}
	if err := tf.P2PAssertPingFromClient(ctx); err != nil {
		s.Fatal("Failed to ping p2p group owner (GO) from the p2p client: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, rt.AP2()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
	if err := tf.VerifyConnectionFromDUT(ctx, wificell.PeerDUT1, rt.AP2()); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
}
