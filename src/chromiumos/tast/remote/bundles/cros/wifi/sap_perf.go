// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/wifi/security"
	"chromiumos/tast/common/wifi/security/wpa"
	"chromiumos/tast/remote/network/iperf"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/remote/wificell/dutcfg"
	"chromiumos/tast/remote/wificell/tethering"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type sapPerfTestcase struct {
	// Any extra options for tethering.
	tetheringOpts []tethering.Option
	// Security facility (nil for Open mode).
	secConfFac security.ConfigFactory
	// TCP vs UDP.
	protocol iperf.Protocol
	// Reverse direction (AP=>STA).
	reverse bool
	// Iperf options.
	opts []iperf.ConfigOption
	// Minimum throughput, set to track regressions: average - 3 * sigma.
	minThroughput iperf.BitRate
}

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPPerf,
		Desc: "Verifies that AP can handle throughput without losses",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"jck@semihalf.com",                // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap", "wificell_cross_device_unstable"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      "wificellFixtCompanionDut",
		Timeout:      15 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.WifiSAP(),
			// Skip test on devices that don't support AP/STA concurrency.
			// TODO(b/223075313) We don't do this globally, because AVL hasn't changed (yet) and we don't want to impact SAPCaps test.
			hwdep.SkipOnWifiDevice(
				hwdep.Realtek8822CPCIE, hwdep.Realtek8852APCIE,
				hwdep.QualcommWCN6855,
			)),
		Params: []testing.Param{
			{
				// TCP performance. Download|Upload directions based on the STA perspective.
				Name: "upload_tcp",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					// TODO(b/269164431): adjust per channel BW and MCS.
					minThroughput: 125 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}},
			},
			{
				// TCP performance, AP->STA direction.
				Name: "download_tcp",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}},
			},
			{
				// UDP performance, STA->AP direction.
				Name: "upload_udp",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}},
			},
			{
				// UDP performance, AP->STA direction.
				Name: "download_udp",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}},
			},
			{
				// Small packets UDP performance (e.g. for games), STA->AP direction.
				Name: "upload_udp_small",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
				}},
			},
			{
				// Small packets UDP performance, AP->STA direction.
				Name: "download_udp_small",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
				}, {
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
				}},
			},
		},
	})
}

func SAPPerf(ctx context.Context, s *testing.State) {
	/*
		This test checks throughput performance of the chromebook by using
		the following steps:
		1- Configures the main DUT as a soft AP.
		2- Configures the Companion DUT as a STA.
		3- Connects the the STA to the soft AP.
		4- Verify the connection by running iperf test.
		5- Deconfigure the STA.
		6- Deconfigure the soft AP.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test requires at least 2 DUTs to be declared. Only have ", tf.NumberOfDUTs())
	}

	// Setup AP to provide reference regulatory domain for all testcases.
	apIface, cancelF, err := tf.SeedRegdomain(ctx, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	defer cancelF(ctx)

	ctx, cancel := tf.ReserveForDeconfigAP(ctx, apIface)
	defer cancel()

	testOnce := func(ctx context.Context, s *testing.State, tc sapPerfTestcase) {
		tetheringConf, _, err := tf.StartTethering(ctx, wificell.DefaultDUT, tc.tetheringOpts, tc.secConfFac)
		if err != nil {
			s.Fatal("Failed to start tethering session on DUT, err: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.StopTethering(ctx, wificell.DefaultDUT); err != nil {
				s.Error("Failed to stop tethering session on DUT, err: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForStopTethering(ctx)
		defer cancel()
		s.Log("Tethering session started")

		cdIdx := wificell.DutIdx(1)
		_, err = tf.ConnectWifiFromDUT(ctx, cdIdx, tetheringConf.SSID, dutcfg.ConnSecurity(tetheringConf.SecConf))
		if err != nil {
			s.Fatal("Failed to connect to Soft AP, err: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DisconnectDUTFromWifi(ctx, cdIdx); err != nil {
				s.Error("Failed to disconnect from Soft AP, err: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		s.Log("Connected")

		pr, err := tf.SAPPerf(ctx, tc.protocol, tc.reverse, tc.opts...)
		if err != nil {
			s.Fatal("Failed to run the performance verification: ", err)
		}

		if pr.Throughput < tc.minThroughput {
			s.Fatalf("Unacceptable throughput in performance test. Wanted > %v, got %v", tc.minThroughput, pr.Throughput)
		}
		if pr.PercentLoss > 5.0 {
			s.Fatalf("Unacceptable loss in performance test. Wanted <= 5%%, got %f", pr.PercentLoss)
		}
	}

	testcases := s.Param().([]sapPerfTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d", i), subtest)
	}
	s.Log("Tearing down")
}
