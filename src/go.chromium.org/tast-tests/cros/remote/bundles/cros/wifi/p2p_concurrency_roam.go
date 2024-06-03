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
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2412)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_same_chan_5g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.Channel(36), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_diff_chan_2g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.Channel(36), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40)}},
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				Name: "chromebook_chromebook_diff_chan_5g",
				Val: p2pConcurrencyRoamTestcase{
					ap1Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211acPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.VHTChWidth(hostapd.VHTChWidth20Or40)}},
					ap2Config: hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}},
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

	verifyConnections := func(dut1, dut2 wificell.DutIdx, ap1, ap2 *wificell.APIface) {
		if err := tf.P2PAssertPingFromGO(ctx); err != nil {
			s.Fatal("Failed to ping the p2p client from the p2p group owner (GO): ", err)
		}
		if err := tf.P2PAssertPingFromClient(ctx); err != nil {
			s.Fatal("Failed to ping p2p group owner (GO) from the p2p client: ", err)
		}
		if err := tf.VerifyConnectionFromDUT(ctx, dut1, ap1); err != nil {
			s.Fatal("Failed to verify connection: ", err)
		}
		if err := tf.VerifyConnectionFromDUT(ctx, dut2, ap2); err != nil {
			s.Fatal("Failed to verify connection: ", err)
		}
	}

	doRun := func(ctx context.Context) error {
		verifyConnections(wificell.DefaultDUT, wificell.PeerDUT1, rt.AP1(), rt.AP1())

		fromBSSID := rt.AP1BSSID()
		roamBSSID := rt.AP2BSSID()
		testSSID := rt.AP1SSID()
		s.Log("AP 1 BSSID: ", fromBSSID)
		s.Log("AP 2 BSSID: ", roamBSSID)

		var requestParams hostapd.BSSTMReqParams
		requestParams.Neighbors = []string{roamBSSID}
		if err := rt.SetupDUTForRoaming(ctx, wificell.DefaultDUT, fromBSSID, roamBSSID, testSSID, true); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		if err := rt.SendBSSTMReqAndWaitConnected(ctx, wificell.DefaultDUT, fromBSSID, roamBSSID, rt.AP1(), rt.AP2(), requestParams, rt.ServicePathOfDUT(wificell.DefaultDUT), false); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		verifyConnections(wificell.DefaultDUT, wificell.PeerDUT1, rt.AP2(), rt.AP1())

		if err := rt.SetupDUTForRoaming(ctx, wificell.PeerDUT1, fromBSSID, roamBSSID, testSSID, true); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		if err := rt.SendBSSTMReqAndWaitConnected(ctx, wificell.PeerDUT1, fromBSSID, roamBSSID, rt.AP1(), rt.AP2(), requestParams, rt.ServicePathOfDUT(wificell.PeerDUT1), false); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		verifyConnections(wificell.DefaultDUT, wificell.PeerDUT1, rt.AP2(), rt.AP2())

		return nil
	}

	if err := tf.P2PAssertNoDisconnect(ctx, wificell.DefaultDUT, doRun); err != nil {
		s.Error("Failed to run roaming test, err: ", err)
	}
}
