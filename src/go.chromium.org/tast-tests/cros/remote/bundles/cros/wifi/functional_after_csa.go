// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FunctionalAfterCSA,
		Desc: "Verifies that the DUT can still connect to the AP when it is disconnected right after receiving a CSA message. This is to make sure the MAC 80211 queues are not stuck after receiving CSA and disconnect events consecutively. Refer to crbug.com/408370 for more information to the test description",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "group:release-health", "release-health_wifi"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name:    "client",
				Val:     true,
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
			}, {
				Name: "router",
				Val:  false,
				// TODO(b/197414763): Adding pcap to investigate the failure
				// of disconnecting due to "CLASS3_FRAME_FROM_NONASSOC_STA".
				Fixture: wificell.FixtureID(wificell.TFFeaturesCapture),
			},
		},
	})
}

func FunctionalAfterCSA(ctx context.Context, s *testing.State) {
	/*
		FunctionalAfterCSA tests the functionality after receiving a channel switch announcement (CSA) message
		It runs the following test |numRounds| times
			1. Configure AP on |primaryChannel|
			2. Connect the DUT to the AP
			3. The AP starts channel switch to |alternateChannel| by sending CSA messages
			If the DUT initiates the disconnection
			4. Disconnect WiFi on the DUT
			If the AP initiates the disconnection
			4. The AP sends deauthentication to the DUT
			5. Verify that the DUT is disconnected
			6. Swap |primaryChannel| and |alternateChannel|
	*/
	const (
		numRounds = 5
		csCount   = 10
	)
	var (
		primaryChannel   = 48
		alternateChannel = 36
	)

	tf := s.FixtValue().(*wificell.TestFixture)
	clientInitDisconnect := s.Param().(bool)
	csaDisconnectCore := func(ctx context.Context, primaryChannel, alternateChannel int) {
		s.Logf("Setting up the AP on channel %d", primaryChannel)
		apOps := []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(primaryChannel), hostapd.HTCaps(hostapd.HTCapHT20)}
		ap, err := tf.ConfigureAP(ctx, apOps, nil)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap); err != nil {
				s.Fatal("Failed to deconfig the AP: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		s.Log("Connecting to AP")
		// Disable autoconnect.
		configProps := map[string]interface{}{
			shillconst.ServicePropertyAutoConnect: false,
		}
		resp, err := tf.ConnectWifiAP(ctx, ap, dutcfg.ConnProperties(configProps))
		if err != nil {
			s.Fatal("Failed to connect to WiFi: ", err)
		}
		disconnected := false
		defer func(ctx context.Context) {
			if !disconnected {
				if err := tf.DisconnectWifi(ctx); err != nil {
					s.Error("Failed to disconnect WiFi: ", err)
				}
			}
			req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ap.Config().SSID)}
			if _, err := tf.WifiClient().DeleteEntriesForSSID(ctx, req); err != nil {
				s.Errorf("Failed to remove entries for ssid=%s, err: %v", ap.Config().SSID, err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		s.Logf("Connected. Sending channel switch frame (channel switch to %d)", alternateChannel)

		if err := ap.SendChannelSwitchAnnouncement(ctx, csCount, alternateChannel); err != nil {
			s.Fatal("Failed to send channel switch frame: ", err)
		}

		// GoBigSleepLint. Wait a little while for CSA frames to start actually being transmitted
		if err := testing.Sleep(ctx, 100*time.Millisecond); err != nil {
			s.Fatal("Interrupted while sleeping for CSA frames transmission: ", err)
		}

		if clientInitDisconnect {
			// Client initiated disconnect.
			if err := tf.DisconnectWifi(ctx); err != nil {
				// Do not fail on this error as CSA could trigger
				// disconnection in this test and the service can be
				// inactive at this point.
				s.Log("Failed to disconnect WiFi: ", err)
			}
		} else {
			clientHWAddr, err := tf.ClientHardwareAddr(ctx)
			if err != nil {
				s.Fatal("Failed to get the DUT MAC address: ", err)
			}
			// Router initiated disconnect.
			if err := ap.DeauthenticateClient(ctx, clientHWAddr); err != nil {
				s.Fatal("Failed to disconnect WiFi: ", err)
			}
			// Wait for DUT to disconnect.
			if err := tf.WifiClient().AssureDisconnect(ctx, resp.ServicePath, 20*time.Second); err != nil {
				s.Fatalf("DUT: failed to disconnect in %s: %v", 20*time.Second, err)
			}
		}
		disconnected = true
	}

	// Run it multiple times to reproduce the race condition that triggers crbug.com/408370.
	// Alternate the AP channel with the CSA announced channel to work around with drivers
	// (Marvell 8897) that disallow reconnecting immediately to the same AP on the same channel
	// after CSA to a different channel.
	for i := 1; i <= numRounds; i++ {
		s.Logf("Run number %d", i)
		csaDisconnectCore(ctx, primaryChannel, alternateChannel)
		// Swap primaryChannel with alternateChannel so we don't configure
		// AP using same channel in back-to-back runs.
		alternateChannel, primaryChannel = primaryChannel, alternateChannel
	}
}
