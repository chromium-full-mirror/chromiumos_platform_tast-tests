// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DuplicateBSSID,
		Desc: "Test that two APs with the same BSSID, but with different SSIDs can both be seen in the scan results",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "group:release-health", "release-health_wifi"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesRouters),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func DuplicateBSSID(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	// Generate the shared BSSID.
	bssid, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}

	// Create an AP on each router, manually specifying both the SSID and BSSID.
	// Router 0 runs on channel 1, Router 1 runs on channel 36 with the same BSSID
	// but different SSIDs. These APs together are meant to emulate situations
	// that occur with some types of APs which broadcast or respond with more
	// than one (non-empty) SSID across bands.
	type apParams struct {
		routerIdx wificell.RouterIdx
		channel   int
	}
	routers := []apParams{
		{routerIdx: 0, channel: 1},
		{routerIdx: 1, channel: 36},
	}
	var aps []*wificell.APIface
	for _, r := range routers {
		s.Logf("Setting up the AP on router %d, channel %d", r.routerIdx, r.channel)
		options := []hostapd.Option{
			hostapd.Mode(hostapd.Mode80211nPure),
			hostapd.Channel(r.channel),
			hostapd.HTCaps(hostapd.HTCapHT20),
			hostapd.BSSID(bssid.String()),
		}
		ap, err := tf.ConfigureAPOnRouterID(ctx, r.routerIdx, options, nil, false, false)
		if err != nil {
			s.Fatalf("Failed to set up AP on router %d: %v", r.routerIdx, err)
		}
		aps = append(aps, ap)
		defer func(ctx context.Context, ap *wificell.APIface, r apParams) {
			s.Logf("Deconfiguring the AP on router %d, channel %d", r.routerIdx, r.channel)
			if err := tf.DeconfigAP(ctx, ap); err != nil {
				s.Error("Failed to deconfig AP: ", err)
			}
		}(ctx, ap, r)
		var cancel context.CancelFunc
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()
	}

	for _, ap := range aps {
		if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
			s.Errorf("Failed to connect to WiFi SSID %s: %v", ap.Config().SSID, err)
			continue
		}
		if err := tf.PingFromDUT(ctx, ap.ServerIP().String()); err != nil {
			s.Error("Failed to ping from the DUT: ", err)
		}
		if err := tf.DisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}
}
