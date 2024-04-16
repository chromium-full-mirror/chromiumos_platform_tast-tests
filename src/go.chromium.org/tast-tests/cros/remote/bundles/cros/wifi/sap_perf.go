// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sapPerfTestcase struct {
	// Testcase name to be used in perf.
	printableName string
	// Any extra options for tethering.
	tetheringOpts []tethering.Option
	// Security facility (nil for Open mode).
	secConfFac security.ConfigFactory
	// Use wpa_cli API to setup tethering.
	useWpaCliAPI bool
	// TCP vs UDP.
	protocol iperf.Protocol
	// Reverse direction (AP=>STA).
	reverse bool
	// Iperf options.
	opts []iperf.ConfigOption
	// Minimum throughput, set to track regressions: average - 3 * sigma.
	minThroughput iperf.BitRate
	// Maximum jitter.
	maxJitter time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPPerf,
		Desc: "Verifies that AP can handle throughput without losses",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"jsiuda@google.com",               // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap", "wificell_cross_device_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates},
		Timeout:      15 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Params: []testing.Param{
			{
				// TCP performance. Download|Upload directions based on the STA perspective.
				Name: "upload_tcp",
				Val: []sapPerfTestcase{{
					printableName: "open_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					printableName: "open_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					// TODO(b/269164431): adjust per channel BW and MCS.
					minThroughput: 125 * iperf.Mbps,
				}, {
					printableName: "wpa2_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolTCP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					printableName: "wpa2_5",
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
					printableName: "open2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 95 * iperf.Mbps,
				}, {
					printableName: "open_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolTCP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
				}, {
					printableName: "wpa2_2_4",
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
					printableName: "wpa2_5",
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
					printableName: "open2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "open5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}},
			},
			{
				// UDP performance, AP->STA direction.
				Name: "download_udp",
				Val: []sapPerfTestcase{{
					printableName: "open2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "open5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 100 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{},
					minThroughput: 125 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}},
			},
			{
				// Small packets UDP performance (e.g. for games), STA->AP direction.
				Name: "upload_udp_small",
				Val: []sapPerfTestcase{{
					printableName: "open2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "open5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}},
			},
			{
				// Small packets UDP performance, AP->STA direction.
				Name: "download_udp_small",
				Val: []sapPerfTestcase{{
					printableName: "open2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "open5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_2_4",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 40 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}, {
					printableName: "wpa2_5",
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					protocol:      iperf.ProtocolUDP,
					reverse:       true,
					opts:          []iperf.ConfigOption{iperf.DatagramLengthOption(112 * iperf.B)},
					minThroughput: 65 * iperf.Mbps,
					maxJitter:     10 * time.Millisecond,
				}},
			},
		},
	})
}

func SAPPerf(ctx context.Context, s *testing.State) {
	/*
		This test checks throughput performance of the chromebook by using
		the following steps:
		1- Disable the station interface.
		2- Configures the main DUT as a soft AP.
		3- Configures the Companion DUT as a STA.
		4- Connects the the STA to the soft AP.
		5- Verify the connection by running iperf test.
		6- Deconfigure the STA.
		7- Deconfigure the soft AP.
		8- Re-enable the station interface.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test requires at least 2 DUTs to be declared. Only have ", tf.NumberOfDUTs())
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()
	// Add a cascading timeout to let test store perf report, but don't impact test teardown if it takes too long.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testOnce := func(ctx context.Context, s *testing.State, tc sapPerfTestcase) {
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
		s.Log("Tethering session started")

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

		pr, err := tf.SAPPerf(ctx, tc.protocol, tc.reverse, tc.opts...)
		if err != nil {
			s.Fatal("Failed to run the performance verification: ", err)
		}

		if pr.Throughput < tc.minThroughput {
			s.Fatalf("Unacceptable throughput in performance test. Wanted > %v Mbps, got %v Mbps", tc.minThroughput/iperf.Mbps, pr.Throughput/iperf.Mbps)
		}
		if pr.PercentLoss > 5.0 {
			s.Fatalf("Unacceptable loss in performance test. Wanted <= 5%%, got %f", pr.PercentLoss)
		}

		// Store perf metrics. Using only tc.printableName is enough because results of each subtest are stored in a separate directory.
		pv.Set(perf.Metric{
			Name:      "throughput_" + tc.printableName,
			Unit:      "Mbps",
			Direction: perf.BiggerIsBetter,
		}, math.Round(float64(pr.Throughput/iperf.Mbps))) // Rounding to get rid of the excess of non-significant data, e.g. 184.026360 Mbit/s.
		pv.Set(perf.Metric{
			Name:      "loss_" + tc.printableName,
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}, float64(pr.PercentLoss))

		// If maxJitter is set, it means that we need jitter results.
		if tc.maxJitter > 0 {
			if len(pr.Jitter) == 0 {
				s.Fatal("No jitter results")
			}
			sort.Slice(pr.Jitter, func(i, j int) bool {
				return pr.Jitter[i] < pr.Jitter[j]
			})
			// Pick 90th percentile value. We want to exclude top 10% of recorded jitters,
			// so we can be pretty convinced that 90% of our traffic fits under the maximum acceptable jitter threshold.
			jitter := pr.Jitter[(len(pr.Jitter)-1)*9/10]
			s.Logf("90th percentile jitter: %vus", jitter.Microseconds())
			if jitter >= tc.maxJitter {
				s.Fatalf("Unacceptable jitter in performance test. Wanted < %dus, got %dus", tc.maxJitter.Microseconds(), jitter.Microseconds())
			}
			pv.Set(perf.Metric{
				Name:      "jitter_" + tc.printableName,
				Unit:      "us",
				Direction: perf.SmallerIsBetter,
			}, float64(jitter.Microseconds()))
		}
	}

	testcases := s.Param().([]sapPerfTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d: %s", i+1, len(testcases), tc.printableName), subtest)
	}
	s.Log("Tearing down")
}
