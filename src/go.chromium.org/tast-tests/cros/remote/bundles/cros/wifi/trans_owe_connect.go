// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"strings"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security/owe"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TransOweConnect,
		Desc: "Verifies connectivity to the transitional mode OWE AP",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"andrzejo@google.com",             // Author
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiGenSupportWiFi},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func servicePath(ctx context.Context, tf *wificell.TestFixture, ssid string) (string, error) {
	props := map[string]interface{}{
		shillconst.ServicePropertyType:          shillconst.TypeWifi,
		shillconst.ServicePropertyWiFiHexSSID:   strings.ToUpper(hex.EncodeToString([]byte(ssid))),
		shillconst.ServicePropertySecurityClass: shillconst.SecurityClassNone,
	}
	return tf.WifiClient().GetServicePath(ctx, props)
}

// TransOweConnect tests connectivity to a transitional mode OWE AP.
// This actually is a pair of two APs, an open one "pointing to" the
// hidden one which is using OWE for encryption.
// For more information about OWE please read the go/owe-design doc.
// Step-by-step procedure:
//  1. Configure APs:
//     - first hidden using OWE and pointing in beacon to open BSS,
//     - then open pointing to the hidden one.
//  2. Make sure the open BSS has been seen and that corresponding
//     service has expected security setting.
//  3. Initiate connection to the open BSS.
//  4. Wait for connection to complete and check that BSS connected is
//     the OWE one and security setting is still correct.
//  5. Test connection.
//  6. Disconnect and deconfigure AP(s).
func TransOweConnect(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	apOpts := []ap.Option{ap.Mode(ap.Mode80211g), ap.Channel(1)}
	ssidOpen := ap.RandomSSID("TAST_TEST_OPEN")
	bssOpen, err := ap.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSS: ", err)
	}

	ssidOwe := ap.RandomSSID("TAST_TEST_OWE")
	bssOwe, err := ap.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSS: ", err)
	}

	oweAp, err := tf.ConfigureAP(ctx, append(apOpts, ap.SSID(ssidOwe), ap.BSSID(bssOwe.String())),
		owe.NewConfigFactory(owe.ModeTransOWE, owe.SSID(ssidOpen), owe.BSSID(bssOpen)))
	if err != nil {
		s.Fatal("Failed to configure the OWE AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, oweAp); err != nil {
			s.Error("Failed to deconfig the OWE AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, oweAp)
	defer cancel()

	openAp, err := tf.ConfigureAP(ctx, append(apOpts, ap.SSID(ssidOpen), ap.BSSID(bssOpen.String())),
		owe.NewConfigFactory(owe.ModeTransOpen, owe.SSID(ssidOwe), owe.BSSID(bssOwe)))
	if err != nil {
		s.Fatal("Failed to configure the open AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, openAp); err != nil {
			s.Error("Failed to deconfig the OWE AP: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, openAp)
	defer cancel()

	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get client interface: ", err)
	}

	if err := tf.WifiClient().DiscoverBSSID(ctx, bssOpen.String(), clientIface, []byte(ssidOpen)); err != nil {
		s.Fatal("Failed to discover open AP: ", err)
	}
	srvcPath, err := servicePath(ctx, tf, ssidOpen)
	if err != nil {
		s.Fatal("Failed to get service path: ", err)
	}
	qRsp, err := tf.WifiClient().QueryServiceWithPath(ctx, srvcPath)
	if err != nil {
		s.Fatal("Failed to get service properties: ", err)
	}
	expectedSecurity := shillconst.SecurityTransOWE
	security := qRsp.GetWifi().GetSecurity()
	if security != expectedSecurity {
		s.Fatalf("Wrong security: got %s, want %s", security, expectedSecurity)
	}
	cRsp, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, openAp)
	if err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	if srvcPath != cRsp.ServicePath {
		s.Fatalf("Service paths differ: got %s, want %s", srvcPath, cRsp.ServicePath)
	}
	srvcResp, err := tf.WifiClient().QueryService(ctx)
	if err != nil {
		s.Fatal("Failed to get service properties: ", err)
	}
	s.Log("Connected with Security: ", srvcResp.Wifi.Security)
	if srvcResp.Wifi.Bssid != bssOwe.String() {
		s.Fatalf("Wrong BSS: got %s, want %s", srvcResp.Wifi.Bssid, bssOwe.String())
	}
	if srvcResp.Wifi.Security != expectedSecurity {
		s.Fatalf("Wrong service security: got %s, want %s", srvcResp.Wifi.Security, expectedSecurity)
	}
	// Note that even though we are connected to the OWE BSS we are
	// testing connection to the IP in the "open" network. This is
	// where we wanted to connect, to so traffic should be routed
	// appropriately.
	// Connectivity testing is for 20s to make it longer than any
	// timer possibly running on a service that has connection
	// configured but is not fully completed (e.g. PendingTimer
	// still running).
	ping := func(ctx context.Context) error {
		return tf.PingFromDUT(ctx, openAp.ServerIP().String(), ping.Count(20), ping.Interval(1))
	}
	if err := tf.AssertNoDisconnect(ctx, wificell.DefaultDUT, ping); err != nil {
		s.Fatal("Failed to ping from DUT, err: ", err)
	}
}
