// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"net"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sapSimpleConnectTestcase struct {
	printableName string
	tetheringOpts []tethering.Option
	secConfFac    security.ConfigFactory
	useWpaCliAPI  bool // Use wpa_cli API to setup tethering.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPSimpleConnect,
		Desc: "Verifies that DUT can start a Soft AP interface and STAs with different WiFi configurations can connect to the DUT",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates},
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Params: []testing.Param{
			{
				// Verifies that Soft AP DUT can accept connection from a station with no encryption in low band and high band.
				Name: "open",
				Val: []sapSimpleConnectTestcase{{
					printableName: "LowBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
				}, {
					printableName: "HighBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
				}},
			},
			{
				// Verifies that Soft AP DUT can accept connection from a station with WPA2 PSK encryption in low band and high band.
				Name: "wpa2",
				Val: []sapSimpleConnectTestcase{{
					printableName: "LowBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}, {
					printableName: "HighBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}},
			},
			{
				// Verifies that Soft AP DUT can accept connection from a station with WPA3 PSK encryption in low band and high band.
				Name:              "wpa3",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				Val: []sapSimpleConnectTestcase{{
					printableName: "LowBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}, {
					printableName: "HighBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}},
			},
			{
				// Verifies that Soft AP DUT can accept connection from a station with WPA3 transitional PSK encryption in low band and high band.
				Name:              "wpa3mixed",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				Val: []sapSimpleConnectTestcase{{
					printableName: "LowBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModeMixedWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}, {
					printableName: "HighBand",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModeMixedWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
				}},
			},
		},
	})
}

func SAPSimpleConnect(ctx context.Context, s *testing.State) {
	/*
		This test checks the soft AP connection of the chromebook by using
		the following steps:
		1- Disable the station interface.
		2- Configures the main DUT as a soft AP.
		3- Configures the Companion DUT as a STA.
		4- Connects the the STA to the soft AP.
		5- Verify the connection by running ping from the STA.
		6- Deconfigure the STA.
		7- Deconfigure the soft AP.
		8- Re-enable the station interface.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test requires at least 2 DUTs to be declared. Only have ", tf.NumberOfDUTs())
	}

	testOnce := func(ctx context.Context, s *testing.State, tc sapSimpleConnectTestcase) {
		tf.UseWpaCliAPI(tc.useWpaCliAPI)
		iface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
		if err != nil {
			s.Fatal("DUT: failed to get the client WiFi interface, err: ", err)
		}
		tetheringConf, _, err := tf.StartTethering(ctx, wificell.DefaultDUT, append([]tethering.Option{tethering.PriIface(iface)}, tc.tetheringOpts...), tc.secConfFac)
		if err != nil {
			s.Fatal("Failed to start tethering session on DUT, err: ", err)
		}

		defer func(ctx context.Context) {
			if _, err := tf.StopTethering(ctx, wificell.DefaultDUT, tetheringConf); err != nil {
				s.Error("Failed to stop tethering session on DUT, err: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForStopTethering(ctx)
		defer cancel()

		_, err = tf.ConnectWifiFromDUT(ctx, wificell.PeerDUT1, tetheringConf.SSID, dutcfg.ConnSecurity(tetheringConf.SecConf))
		if err != nil {
			s.Fatal("Failed to connect to Soft AP, err: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DisconnectDUTFromWifi(ctx, wificell.PeerDUT1); err != nil {
				s.Error("Failed to disconnect from Soft AP, err: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		s.Log("Connected")

		addrsReq := &wifi.GetIPv4AddrsRequest{
			InterfaceName: shillconst.ApInterfaceName,
		}
		addrsResp, err := tf.DUTWifiClient(wificell.DefaultDUT).GetIPv4Addrs(ctx, addrsReq)
		if err != nil {
			s.Fatal("Failed to get the IPv4 addresses: ", err)
		}
		if len(addrsResp.Ipv4) == 0 {
			s.Fatal("No IP address returned")
		}
		addr, _, err := net.ParseCIDR(addrsResp.Ipv4[0])
		if err != nil {
			s.Fatalf("Failed to parse IP address %s: %v", addrsResp.Ipv4[0], err)
		}

		if _, err := tf.PingFromSpecificDUT(ctx, wificell.PeerDUT1, addr.String(), ping.Interval(0.1)); err != nil {
			s.Fatal("Failed to ping from Companion DUT to DUT: ", err)
		}
	}

	testcases := s.Param().([]sapSimpleConnectTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d: %s", i+1, len(testcases), tc.printableName), subtest)
	}
	s.Log("Tearing down")
}
