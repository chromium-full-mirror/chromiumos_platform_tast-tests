// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: BSSIDRequestedUpdate,
		Desc: "Checks BSSIDRequested can be updated as part of the same connection",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
	})
}

// BSSIDRequestedUpdate will update the BSSIDRequested service property of an existing network connection.
//
// Concretely, this test:
//  1. Sets up the first AP
//  2. Connects to the first AP without BSSIDRequested
//  3. Sets up the second AP
//  4. Connects to the second AP by specifying the second AP's BSSID as the BSSIDRequested
//  5. Sets the BSSIDRequested back to the first AP's BSSID
//  6. Cleans up
func BSSIDRequestedUpdate(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	ssid := hostapd.RandomSSID("BSSIDRequestedUpdate_")

	// Generate BSSID for ap1.
	ap1HwAddr, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	ap1BSSID := ap1HwAddr.String()

	// Generate different BSSID for ap2.
	ap2HwAddr := ap1HwAddr
	ap2HwAddr[5]++
	ap2BSSID := ap2HwAddr.String()

	// Configure ap1.
	s.Logf("Starting the first AP at %s", ap1BSSID)
	ap1Ops := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.Channel(1),
		hostapd.HTCaps(hostapd.HTCapHT20),
		hostapd.SSID(ssid),
		hostapd.BSSID(ap1BSSID),
	}
	ap1, err := tf.ConfigureAP(ctx, ap1Ops, nil)
	if err != nil {
		s.Fatal("Failed to set up the first AP: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Deconfiguring the first AP")
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig the first AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()

	// Connect to first AP without specifying a BSSIDRequested.
	s.Log("Connecting to first AP")
	configProps := map[string]interface{}{}
	_, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap1, dutcfg.ConnProperties(configProps))
	if err != nil {
		s.Fatal("Failed to connect to first AP without BSSIDRequested: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	if err := pollUntilBSSIDMatches(ctx, tf, ap1BSSID); err != nil {
		s.Fatal("Failed to associate to first BSSID: ", err)
	}

	// Configure ap2.
	s.Logf("Starting the second AP at %s", ap2BSSID)
	ap2Ops := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.Channel(48),
		hostapd.HTCaps(hostapd.HTCapHT20),
		hostapd.SSID(ssid),
		hostapd.BSSID(ap2BSSID),
	}
	ap2, err := tf.ConfigureAP(ctx, ap2Ops, nil)
	if err != nil {
		s.Fatal("Failed to set up the second AP: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Deconfiguring the second AP")
		if err := tf.DeconfigAP(ctx, ap2); err != nil {
			s.Error("Failed to deconfig the second AP: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap2)
	defer cancel()

	// Update connected service's BSSIDRequested property to |ap2BSSID|.
	s.Log("Setting BSSIDRequested to second AP")
	if err := tf.WifiClient().SetBSSIDRequested(ctx, ap2BSSID); err != nil {
		s.Fatalf("Failed to set BSSIDRequested to %s: %s", ap2BSSID, err)
	}
	if err := pollUntilBSSIDMatches(ctx, tf, ap2BSSID); err != nil {
		s.Fatal("Failed to associate to second BSSID: ", err)
	}

	// Update connected service's BSSIDRequested property back to |ap1BSSID|.
	s.Log("Setting BSSIDRequested back to first AP")
	if err := tf.WifiClient().SetBSSIDRequested(ctx, ap1BSSID); err != nil {
		s.Fatalf("Failed to set BSSIDRequested to %s: %s", ap1BSSID, err)
	}
	if err := pollUntilBSSIDMatches(ctx, tf, ap1BSSID); err != nil {
		s.Fatal("Failed to associate to first BSSID: ", err)
	}
}

func pollUntilBSSIDMatches(ctx context.Context, tf *wificell.TestFixture, bssid string) error {
	if err := tf.WifiClient().RequestScan(ctx); err != nil {
		return errors.Wrap(err, "failed to request scan")
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := tf.WifiClient().QueryService(ctx)
		if err != nil || resp.Wifi.Bssid != bssid {
			return errors.Wrap(err, "still not associated to bssid")
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return errors.Wrapf(err, "failed to reassociate to BSSID %s", bssid)
	}
	return nil
}
