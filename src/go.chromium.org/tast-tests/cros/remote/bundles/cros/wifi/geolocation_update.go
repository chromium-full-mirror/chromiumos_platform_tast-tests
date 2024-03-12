// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GeolocationUpdate,
		Desc: "Test the geolocation update after scan is done",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func", "wificell_suspend"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
	})
}

/*
	This test verifies a device updates the geolocation cache using the latest
	scan results after resume. It is expected that a device updates the
	geolocation cache after each WiFi scan, either foreground scan or background
	scan.

	Here are the full steps this test takes:
	1- Set up an open AP "AP 0".
	2- Connect to AP 0.
	3- Request geolocation cache from the DUT.
	4- Verify that AP 0 is in the geolocation cache, while AP 1 is not.
	5- Set up another open AP "AP 1".
	6- Request geolocation cache from the DUT.
	7- Verify that AP 0 is in the geolocation cache, while AP 1 is not.
	8- Suspend and resume the DUT.
	9- Wait until the DUT auto connects to AP 0.
	10- Request geolocation cache from the DUT.
	11- Verify that AP 0 and AP 1 are both in the geolocation cache.
*/

func GeolocationUpdate(ctx context.Context, s *testing.State) {
	const (
		// wpa_supplicant/config.h defines scan_res_valid_for_connect, which
		// configures the number of seconds old scan results are considered
		// valid for association. When scan results are older than this value a
		// new scan is triggered prior to the association.
		// suspendTime needs to be larger than its default value (5 seconds) so
		// that wpa_supplicant doesn't skip the scan.
		suspendTime = 10 * time.Second
	)

	tf := s.FixtValue().(*wificell.TestFixture)

	apOps := []hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211g)}
	ap0, err := tf.ConfigureAP(ctx, apOps, nil)
	if err != nil {
		s.Fatal("Failed to configure AP 0: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap0); err != nil {
			s.Error("Failed to deconfig AP 0: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap0)
	defer cancel()

	s.Log("AP 0 setup done; connecting")

	if _, err := tf.ConnectWifiAP(ctx, ap0); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	if err := tf.PingFromDUT(ctx, ap0.ServerIP().String()); err != nil {
		s.Error("Failed to ping from the DUT: ", err)
	}

	ap1BSSID, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate BSSID for AP 1")
	}

	geolocationInfos, err := tf.WifiClient().GetWiFiNetworksForGeolocation(ctx)
	if err != nil {
		s.Fatal("Failed to request GetWiFiNetworksForGeolocation: ", err)
	}
	if !findBSS(geolocationInfos, ap0.Config().BSSID) {
		s.Fatal("Failed to discover AP 0 in geolocation cache")
	}
	s.Log("AP 0 discovered in the geolocation cache")
	if findBSS(geolocationInfos, ap1BSSID.String()) {
		s.Fatal("AP 1 discovered in the geolocation cache")
	}
	s.Log("AP 1 not in the geolocation cache")

	ap1ps := []hostapd.Option{hostapd.Channel(36), hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.BSSID(ap1BSSID.String())}
	ap1, err := tf.ConfigureAP(ctx, ap1ps, nil)
	if err != nil {
		s.Fatal("Failed to configure AP 1: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig AP 1: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()
	s.Log("AP 1 setup done")

	geolocationInfos, err = tf.WifiClient().GetWiFiNetworksForGeolocation(ctx)
	if err != nil {
		s.Fatal("Failed to request GetWiFiNetworksForGeolocation: ", err)
	}
	if !findBSS(geolocationInfos, ap0.Config().BSSID) {
		s.Fatal("Failed to discover AP 0 in geolocation cache")
	}
	s.Log("AP 0 discovered in the geolocation cache")
	if findBSS(geolocationInfos, ap1.Config().BSSID) {
		s.Fatal("AP 1 discovered in the geolocation cache")
	}
	s.Log("AP 1 not in the geolocation cache")

	s.Log("Suspend the DUT for ", suspendTime)
	if _, err := tf.WifiClient().SuspendAssertConnect(ctx, suspendTime); err != nil {
		s.Error("Failed to assert WiFi connection after suspend-resume: ", err)
	} else {
		s.Log("WiFi connected after suspend-resume")
	}
	geolocationInfos, err = tf.WifiClient().GetWiFiNetworksForGeolocation(ctx)
	if err != nil {
		s.Fatal("Failed to request GetWiFiNetworksForGeolocation: ", err)
	}
	if !findBSS(geolocationInfos, ap0.Config().BSSID) {
		s.Fatal("Failed to discover AP 0 in geolocation cache")
	}
	s.Log("AP 0 discovered in the geolocation cache")
	if !findBSS(geolocationInfos, ap1.Config().BSSID) {
		s.Fatal("Failed to discover AP 1 in geolocation cache")
	}
	s.Log("AP 1 discovered in the geolocation cache")
}

func findBSS(wifiNetworks *wifi.NetworksForGeolocation, bssid string) bool {
	geolocationInfoList := wifiNetworks.GetGeolocationInfoList()
	for _, geoInfo := range geolocationInfoList {
		info := geoInfo.GetInfo()
		macAddress, ok := info[shillconst.GeoInfoGeoMacAddressProperty]
		if !ok {
			continue
		}
		if bssid == macAddress.GetStr() {
			return true
		}
	}
	return false
}
