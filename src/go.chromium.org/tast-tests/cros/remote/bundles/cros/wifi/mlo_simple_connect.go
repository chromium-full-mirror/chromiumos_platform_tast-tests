// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/mtk"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/mtk/wireless"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type mloSimpleConnectTestcase struct {
	apConfig         wireless.Config
	secConfFac       security.ConfigFactory
	expectedSecurity string
}

var (
	wifiDeviceConfig2G = wireless.DeviceConfig{Name: wireless.WiFiDevice2G, Channel: 1, Disabled: false, Band: "2.4G", Country: "US", HtMode: "EHT40", HtCoex: true}
	wifiDeviceConfig5G = wireless.DeviceConfig{Name: wireless.WiFiDevice5G, Channel: 36, Disabled: false, Band: "5G", Country: "US", HtMode: "EHT160"}
	wifiDeviceConfig6G = wireless.DeviceConfig{Name: wireless.WiFiDevice6G, Channel: 5, Disabled: false, Band: "6G", Country: "US", HtMode: "EHT320"}
	wifiIfaceConfig2G  = wireless.IfaceConfig{Name: wireless.WiFiIface2G, Disabled: false, VifIdx: 1, Device: wireless.WiFiDevice2G, Network: "lan", Mode: "ap", Ieee80211w: 2}
	wifiIfaceConfig5G  = wireless.IfaceConfig{Name: wireless.WiFiIface5G, Disabled: false, VifIdx: 1, Device: wireless.WiFiDevice5G, Network: "lan", Mode: "ap", Ieee80211w: 2}
	wifiIfaceConfig6G  = wireless.IfaceConfig{Name: wireless.WiFiIface6G, Disabled: false, VifIdx: 1, Device: wireless.WiFiDevice6G, Network: "lan", Mode: "ap", Ieee80211w: 2}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MLOSimpleConnect,
		Desc: "Verifies that DUT can connect to the MLO network",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"kaidong@google.com",              // Test author
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Requirements:    []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				// Verifies that DUT can connect to 802.11be MLO network of 2.4 GHz and 5 GHz.
				Name:    "2g5g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloSimpleConnectTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wifiDeviceConfig2G, wifiDeviceConfig5G, wifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wifiIfaceConfig2G, wifiIfaceConfig5G, wifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface5G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 2}},
					},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					expectedSecurity: shillconst.SecurityWPA3,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				// Verifies that DUT can connect to 802.11be MLO network of 2.4 GHz and 6 GHz.
				Name:    "2g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloSimpleConnectTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wifiDeviceConfig2G, wifiDeviceConfig5G, wifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wifiIfaceConfig2G, wifiIfaceConfig5G, wifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 2}},
					},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					expectedSecurity: shillconst.SecurityWPA3,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				// Verifies that DUT can connect to 802.11be MLO network of 5 GHz and 6 GHz.
				Name:    "5g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloSimpleConnectTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wifiDeviceConfig2G, wifiDeviceConfig5G, wifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wifiIfaceConfig2G, wifiIfaceConfig5G, wifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface5G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 2}},
					},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					expectedSecurity: shillconst.SecurityWPA3,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
			{
				// Verifies that DUT can connect to 802.11be MLO network of 2.4 GHz, 5 GHz and 6 GHz.
				Name:    "2g5g6g",
				Fixture: wificell.FixtureID(wificell.TFFeaturesNone),
				Val: mloSimpleConnectTestcase{
					apConfig: wireless.Config{
						DeviceConfigs: []wireless.DeviceConfig{wifiDeviceConfig2G, wifiDeviceConfig5G, wifiDeviceConfig6G},
						IfaceConfigs:  []wireless.IfaceConfig{wifiIfaceConfig2G, wifiIfaceConfig5G, wifiIfaceConfig6G},
						MldConfigs: []wireless.MldConfig{{Name: "apmld1", Disabled: false, Mode: "ap", Ifaces: []string{wireless.WiFiIface2G, wireless.WiFiIface5G, wireless.WiFiIface6G},
							SSID: hostapd.RandomSSID("MLO_"), Encryption: "sae", Key: "chromeos", Ieee80211w: 2}},
					},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					expectedSecurity: shillconst.SecurityWPA3,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211be()),
			},
		},
	})
}

func MLOSimpleConnect(ctx context.Context, s *testing.State) {
	const defaultGatewayIP = "192.168.1.1"
	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(mloSimpleConnectTestcase)
	rt, ok := tf.Router().(*mtk.Router)
	if !ok {
		s.Fatal("Router used is not the MTK 7988 router")
	}
	ws, err := rt.StartWireless(ctx, "", &tc.apConfig)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := rt.StopWireless(ctx, ws); err != nil {
			s.Error("Failed to deconfig ap, err: ", err)
		}
	}(ctx)
	ctx, cancel := ws.ReserveForClose(ctx)
	defer cancel()
	s.Log("AP setup done")

	defer func(ctx context.Context) {
		req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ws.Config().MldConfigs[0].SSID)}
		if _, err := tf.WifiClient().DeleteEntriesForSSID(ctx, req); err != nil {
			s.Errorf("Failed to remove entries for ssid=%s, err: %v", ws.Config().MldConfigs[0].SSID, err)
		}
	}(ctx)

	securityConfig, err := tc.secConfFac.Gen()
	if err != nil {
		s.Fatal("Fail to generate scurity config")
	}
	opts := dutcfg.ConnSecurity(securityConfig)
	if _, err := tf.ConnectWifiFromDUT(ctx, wificell.DefaultDUT, ws.Config().MldConfigs[0].SSID, opts); err != nil {
		s.Fatal("Failed to connect to WiFi, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	s.Log("Connected")

	// Get "iw dev interface link" output
	iwr := iw.NewRemoteRunner(s.DUT().Conn())
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get the client interface, err: ", err)
	}
	mloLinks, nss, err := iwr.MLOLinks(ctx, clientIface)
	if err != nil {
		s.Fatal("Failed to get the MLO information, err: ", err)
	}
	if len(mloLinks) != len(tc.apConfig.MldConfigs[0].Ifaces) {
		s.Fatalf("Wrong number of MLO links: got %d, want %d", len(mloLinks), len(tc.apConfig.MldConfigs[0].Ifaces))
	}
	s.Logf("EHT NSS is %d", nss)

	ping := func(ctx context.Context) error {
		return tf.PingFromDUT(ctx, defaultGatewayIP)
	}
	if err := tf.AssertNoDisconnect(ctx, wificell.DefaultDUT, ping); err != nil {
		s.Fatal("Failed to ping from DUT, err: ", err)
	}
	s.Log("Checking the status of the SSID in the DUT")
	serInfo, err := tf.WifiClient().QueryService(ctx)
	if err != nil {
		s.Fatal("Failed to get the WiFi service information from DUT, err: ", err)
	}
	if tc.expectedSecurity == "" {
		tc.expectedSecurity = shillconst.SecurityNone
	}
	if tc.expectedSecurity != serInfo.Wifi.Security {
		s.Fatalf("Wrong security of the service: got %s, want %s ", serInfo.Wifi.Security, tc.expectedSecurity)
	}
	s.Log("Deconfiguring")
}
