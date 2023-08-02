// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SSIDSwitchBack,
		Desc: "Verifies that the DUT can rejoin a previously connected AP when it loses connectivity to its current AP",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
	})
}

func SSIDSwitchBack(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	// We might respawn APs with the same options. Generate BSSIDs
	// by ourselves so that it won't be re-generated and will be
	// fixed in every usage.
	var bssids []string
	for i := 0; i < 2; i++ {
		addr, err := hostapd.RandomMAC()
		if err != nil {
			s.Fatal("Failed to generate BSSID: ", err)
		}
		bssids = append(bssids, addr.String())
	}
	apOps1 := []hostapd.Option{
		hostapd.SSID(hostapd.RandomSSID("SSIDSwitchBack_1_")),
		hostapd.BSSID(bssids[0]),
		hostapd.Mode(hostapd.Mode80211nMixed),
		hostapd.Channel(1),
		hostapd.HTCaps(hostapd.HTCapHT20),
	}
	apOps2 := []hostapd.Option{
		hostapd.SSID(hostapd.RandomSSID("SSIDSwitchBack_2_")),
		hostapd.BSSID(bssids[1]),
		hostapd.Mode(hostapd.Mode80211nMixed),
		hostapd.Channel(6),
		hostapd.HTCaps(hostapd.HTCapHT20),
	}

	servicePath, err := wifiutil.TryConnect(ctx, tf, apOps1)
	if err != nil {
		s.Fatal("Failed to connect to AP1: ", err)
	}
	if _, err := wifiutil.TryConnect(ctx, tf, apOps2); err != nil {
		s.Fatal("Failed to connect to AP2: ", err)
	}

	// Respawn AP1 and see if DUT can reconnect to it.
	ap, err := tf.ConfigureAP(ctx, apOps1, nil)
	if err != nil {
		s.Fatal("Failed to respawn AP1")
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig the respawned AP1: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	// We use CHECK_WAIT here instead of spawning watcher before ConfigureAP for
	// a more precise timeout. (Otherwise, timeout will include the time used
	// by ConfigureAP.)
	s.Log("Waiting for DUT to auto reconnect")
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	props := []*wificell.ShillProperty{{
		Property:       shillconst.ServicePropertyIsConnected,
		ExpectedValues: []interface{}{true},
		Method:         wifi.ExpectShillPropertyRequest_CHECK_WAIT,
	}}
	wait, err := tf.WifiClient().ExpectShillProperty(waitCtx, servicePath, props, nil)
	if err != nil {
		s.Fatal("Failed to watch service state: ", err)
	}
	if _, err := wait(); err != nil {
		s.Fatal("Failed to wait for service connected: ", err)
	}
	// As we get reconnected now, defer clean disconnect.
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	s.Log("Verifying connection")
	if err := tf.VerifyConnection(ctx, ap); err != nil {
		s.Fatal("Failed to verify connection to the respawned AP1: ", err)
	}
}
