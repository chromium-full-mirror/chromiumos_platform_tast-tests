// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/wificell"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type p2pSimpleConnectTestcase struct {
	p2pOpts         []p2p.GroupOption
	p2pGODevice     wificell.P2PDevice
	p2pClientDevice wificell.P2PDevice
}

func init() {
	testing.AddTest(&testing.Test{
		Func: P2PSimpleConnect,
		Desc: "Tests P2P connection between two chromebooks",
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
				// Verifies that the DUT can connect to another DUT p2p group owner on 2.4GHz band.
				Name: "chromebook_chromebook_2_4ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(2462)},
					p2pGODevice:     wificell.P2PDeviceDUT,
					p2pClientDevice: wificell.P2PDeviceCompanionDUT,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				// Verifies that the DUT can connect to another DUT p2p group owner on 5GHz band.
				Name: "chromebook_chromebook_5ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(5180)},
					p2pGODevice:     wificell.P2PDeviceDUT,
					p2pClientDevice: wificell.P2PDeviceCompanionDUT,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
			}, {
				// Verifies that the android device can connect to the DUT group owner  on 2.4GHz band.
				Name: "chromebook_android_2_4ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(2412)},
					p2pGODevice:     wificell.P2PDeviceDUT,
					p2pClientDevice: wificell.P2PDeviceAndroidDevice,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCompanionAndroid),
			}, {
				// Verifies that the android device can connect to the DUT group owner on 5GHz band.
				Name: "chromebook_android_5ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(5180)},
					p2pGODevice:     wificell.P2PDeviceDUT,
					p2pClientDevice: wificell.P2PDeviceAndroidDevice,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCompanionAndroid),
			}, {
				// Verifies that the DUT can connect to the android device group owner on 2.4GHz band.
				Name: "android_chromebook_2_4ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(2412)},
					p2pGODevice:     wificell.P2PDeviceAndroidDevice,
					p2pClientDevice: wificell.P2PDeviceCompanionDUT,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCompanionAndroid),
			}, {
				// Verifies that the DUT can connect to the android device group owner on 5GHz band.
				Name: "android_chromebook_5ghz",
				Val: p2pSimpleConnectTestcase{
					p2pOpts:         []p2p.GroupOption{p2p.SetFreq(5180)},
					p2pGODevice:     wificell.P2PDeviceAndroidDevice,
					p2pClientDevice: wificell.P2PDeviceCompanionDUT,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCompanionAndroid),
			}},
	})
}

func P2PSimpleConnect(ctx context.Context, s *testing.State) {
	/*
		This test checks the p2p connection between two devices by using
		the following steps:
		1- Configures device A as a p2p group owner (GO).
		2- Configures device B as a p2p client.
		3- Connects the p2p client to the GO network.
		5- Verify the p2p connection.
		5-1- Run ping from the p2p GO to the p2p client.
		5-2- Run ping from the p2p client to the p2p GO.
		7- Disconnect p2p client from GO network.
		8- Deconfigure the p2p GO.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	testcase := s.Param().(p2pSimpleConnectTestcase)

	if err := tf.P2PConfigureGO(ctx, testcase.p2pGODevice, testcase.p2pOpts...); err != nil {
		s.Fatal("Failed to configure the p2p group owner (GO): ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.P2PDeconfigureGO(ctx); err != nil {
			s.Error("Failed to deconfigure the p2p group owner (GO): ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigP2P(ctx)
	defer cancel()

	if err := tf.P2PConnect(ctx, testcase.p2pClientDevice); err != nil {
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
}
