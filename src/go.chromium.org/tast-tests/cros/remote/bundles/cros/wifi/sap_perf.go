// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil/perfmanager"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	remoteiw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sapPerfTestThreshold struct {
	throughput iperf.BitRate
	jitter     time.Duration
	lost       float64
}

type sapPerfTestcase struct {
	tetheringOpts []tethering.Option
	secConfFac    security.ConfigFactory
	useWpaCliAPI  bool
	powerSave     bool
}

var (
	jitterThreshold time.Duration = 20 * time.Millisecond
	lostThreshold   float64       = 5
	perfTestTypes                 = []perfmanager.TestType{
		perfmanager.TestTypeTCPTx,
		perfmanager.TestTypeTCPRx,
		perfmanager.TestTypeUDPTx,
		perfmanager.TestTypeUDPRx,
		perfmanager.TestTypeTCPBidirectional,
		perfmanager.TestTypeUDPBidirectional,
		perfmanager.TestTypeUDPTxSmall,
		perfmanager.TestTypeUDPRxSmall,
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPPerf,
		Desc: "Verifies that AP can handle throughput without losses",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"jsiuda@google.com",               // Test author
			"jintaolin@google.com",
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap", "wificell_cross_device_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates},
		Timeout:      20 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Params: []testing.Param{
			{
				Name: "open",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
					useWpaCliAPI:  true,
					powerSave:     false,
				}},
			},
			{
				Name: "wpa2",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA2)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
					),
					powerSave: false,
				}},
			},
			{
				Name: "wpa3",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModePureWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
					powerSave: false,
				}},
			},
			{
				Name: "wpa3mixed",
				Val: []sapPerfTestcase{{
					tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
						tethering.SecMode(wpa.ModeMixedWPA3)},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
					),
					powerSave: false,
				}},
			},
		},
	})
}

func SAPPerf(ctx context.Context, s *testing.State) {
	/*
		This test checks throughput performance of the chromebook by using
		the following steps:
		1- Disable the station interface on the main DUT.
		2- Configures the main DUT as a soft AP.
		3- Configures the Companion DUT as a STA.
		4- Connects the the STA to the soft AP.
		5- Disable bgscan and power save on STA.
		6- Measure the throughput performance on different traffic types/directions.
		7- Deconfigure the STA.
		8- Deconfigure the soft AP.
		9- Re-enable the station interface on the main DUT.
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

	logPerfValues := func(label, unit string, value float64, dir perf.Direction) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      unit,
			Direction: dir,
		}, value)
		s.Logf("%s: %v", label, value)
	}

	// Disable background scan which causes severe udp_rx throughput drops that can cause
	// a disconnection during perf tests on the companion DUT. Refer to b/315880821.
	ctx, restoreBgAndFg, err := tf.DUTWifiClient(wificell.PeerDUT1).TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	// Verify that performance test result passes requirements and save the result.
	verifyResults := func(ctx context.Context, testType perfmanager.TestType, pr *iperf.Result) {
		// TODO(b/316207256) Retrieve PHY spec from connection status.
		expectedThroughput, err := perfmanager.ExpectedThroughputWiFi(support.SoftAPT, testType, hostapd.Mode80211axMixed, hostapd.ChWidth20)
		if err != nil {
			s.Fatal("Failed to get expected throughput")
		}
		if float64(pr.Throughput/iperf.Mbps) < expectedThroughput.Must {
			s.Fatalf("Unacceptable throughput in performance test. Wanted > %v Mbps, got %v Mbps", expectedThroughput.Must, pr.Throughput/iperf.Mbps)
		}

		// Keep jitter and packet lost informational for UDP small packet tests.
		// TODO(b/316207256): Set jitter and lost criteria for these tests based on test results.
		if testType == perfmanager.TestTypeUDPTx || testType == perfmanager.TestTypeUDPRx {
			if pr.PercentLoss > lostThreshold {
				s.Fatalf("Unacceptable loss in performance test. Wanted <= %f%%, got %f%%", lostThreshold, pr.PercentLoss)
			}
		}

		if testType == perfmanager.TestTypeUDPTx || testType == perfmanager.TestTypeUDPRx {
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
			if jitter >= jitterThreshold {
				s.Fatalf("Unacceptable jitter in performance test. Wanted < %dus, got %dus", jitterThreshold, jitter.Microseconds())
			}
		}
	}

	testOnce := func(ctx context.Context, s *testing.State, tc sapPerfTestcase) {
		tf.UseWpaCliAPI(tc.useWpaCliAPI)

		apPrimIface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
		if err != nil {
			s.Fatal("DUT: failed to get the client WiFi interface, err: ", err)
		}
		tetheringConf, _, err := tf.StartTethering(ctx, wificell.DefaultDUT, append([]tethering.Option{tethering.PriIface(apPrimIface)}, tc.tetheringOpts...), tc.secConfFac)
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
		// Create apConfigTag which is used in the keyval
		apConfigDesc := tetheringConf.PerfDesc()
		s.Logf("Tethering session started with config: %s", apConfigDesc)

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

		staIface, err := tf.DUTClientInterface(ctx, wificell.PeerDUT1)
		if err != nil {
			s.Fatal("DUT: failed to get the STA WiFi interface, err: ", err)
		}

		// Disable power save on the companion DUT
		iwr := remoteiw.NewRemoteRunner(tf.DUTConn(wificell.PeerDUT1))
		psMode, err := iwr.PowersaveMode(ctx, staIface)
		if err != nil {
			s.Error("Failed to get the powersave mode of the WiFi interface, err: ", err)
		}
		defer func(ctx context.Context) error {
			if err := iwr.SetPowersaveMode(ctx, staIface, psMode); err != nil {
				s.Errorf("Failed to set the powersave mode %t: %v", psMode, err)
			}
			return nil
		}(ctx)
		ctx, cancel = ctxutil.Shorten(ctx, 2*time.Second)
		defer cancel()
		if err := iwr.SetPowersaveMode(ctx, staIface, tc.powerSave); err != nil {
			s.Fatalf("Failed to set the powersave mode %t: %v", tc.powerSave, err)
		}

		apIP, err := tf.DUTIfaceIPv4Addrs(ctx, wificell.DefaultDUT, shillconst.ApInterfaceName)
		if err != nil || len(apIP) == 0 {
			s.Fatal("Failed to get SoftAP IP address, err: ", err)
		}

		staIP, err := tf.DUTIfaceIPv4Addrs(ctx, wificell.PeerDUT1, staIface)
		if err != nil || len(staIP) == 0 {
			s.Fatal("Failed to get STA IP address, err: ", err)
		}

		// 2-way performance setup
		manager, err := perfmanager.NewTestManager(ctx, tf.DUTConn(wificell.PeerDUT1), tf.DUTConn(wificell.DefaultDUT), nil, support.SoftAPT, staIP[0].String(), apIP[0].String(), staIface)
		if err != nil {
			s.Fatal("Failed to get performance test manager, err: ", err)
		}
		defer func(ctx context.Context) error {
			if err := manager.Close(ctx); err != nil {
				s.Error("Failed to Close Perf Test Manager, err: ", err)
			}
			return nil
		}(ctx)
		ctx, cancel = ctxutil.Shorten(ctx, 2*time.Second)

		doRun := func(ctx context.Context) error {
			for _, testType := range perfTestTypes {
				s.Logf("Performing [[ %s ]]", testType)
				config, err := manager.Config(support.SoftAPT, testType, 0)
				if err != nil {
					return errors.Wrapf(err, "failed to get the iperf/netperf configuration for test type %s", testType)
				}
				session, err := manager.Session(ctx, testType)
				if err != nil {
					return errors.Wrapf(err, "failed to get the iperf/netperf session for test type %s", testType)
				}
				pr, _, err := session.Run(ctx, config)
				if err != nil {
					return errors.Wrap(err, "failed to run session")
				}
				logPerfValues(fmt.Sprintf("%s.%s", apConfigDesc, testType), "Mbps", float64(pr.Throughput/iperf.Mbps), perf.BiggerIsBetter)
				verifyResults(ctx, testType, pr)
			}
			return nil
		}
		if err := tf.AssertNoDisconnect(ctx, wificell.PeerDUT1, doRun); err != nil {
			s.Error("Failed run performance test, err: ", err)
		}
		s.Log("Deconfiguring")
	}

	testcases := s.Param().([]sapPerfTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d", i+1, len(testcases)), subtest)
	}
	s.Log("Tearing down")
}
