// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil/perfmanager"
	iperf "go.chromium.org/tast-tests/cros/remote/network/iperf"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type p2pConcurrencyPerfTestcase struct {
	printableName    string
	p2pOpts          []p2p.GroupOption
	apOpts           []ap.Option
	GoDev, ClientDev wificell.P2PDevice
	testType         []perfmanager.TestType
	powerSave        bool
}

var defaultTestTypes []perfmanager.TestType = []perfmanager.TestType{
	perfmanager.TestTypeUDPBidirectional,
	perfmanager.TestTypeTCPRx,
	perfmanager.TestTypeTCPTx,
}

var infraConfigLoBand = []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT20)}
var infraConfigHiBand = []ap.Option{ap.Mode(ap.Mode80211acPure), ap.Channel(48), ap.HTCaps(ap.HTCapHT40), ap.VHTChWidth(ap.VHTChWidth20Or40)}

func init() {
	testing.AddTest(&testing.Test{
		Func: P2PConcurrencyPerf,
		Desc: "Tests the concurrent performance of both WiFi Direct and Infra WiFi",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_p2p", "wificell_cross_device_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
		HardwareDepsForAll: map[string]hwdep.Deps{
			"":    hwdep.D(hwdep.WifiP2P()),
			"cd1": hwdep.D(hwdep.WifiP2P()),
		},
		Requirements: []string{tdreq.WiFiGenSupportWFD},
		Timeout:      60 * time.Minute,
		// The parameter set is based on three variables:
		// 1) Relative channel difference: SCC/MCC/Multi-band.
		// 2) Whether DUT is P2P Group Owner or P2P Client.
		// 3) Traffic type (TCP/UDP):
		//    bidirectional UDP is used to assess general stack performance, TX/RX TCP match BeTo use cases..
		Params: []testing.Param{
			{
				// Checks performance when DUT connects to AP and is p2p GO on 2GHz band on different channels.
				Name: "different_channel_2ghz_go",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceDUT,
					ClientDev: wificell.P2PDeviceCompanionDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
					apOpts:    infraConfigLoBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP and is a p2p client on 2GHz band on different channels.
				Name: "different_channel_2ghz_client",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceCompanionDUT,
					ClientDev: wificell.P2PDeviceDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
					apOpts:    infraConfigLoBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP and is p2p GO on 5GHz band on different channels.
				Name: "different_channel_5ghz_go",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceDUT,
					ClientDev: wificell.P2PDeviceCompanionDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
					apOpts:    infraConfigHiBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP and is a p2p client on 5GHz band on different channels.
				Name: "different_channel_5ghz_client",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceCompanionDUT,
					ClientDev: wificell.P2PDeviceDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
					apOpts:    infraConfigHiBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP on 2Ghz band and is p2p GO on 5GHz band.
				Name: "different_bands_2_plus_5_go",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceDUT,
					ClientDev: wificell.P2PDeviceCompanionDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
					apOpts:    infraConfigLoBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP on 2Ghz band and is a p2p client on 5Ghz band.
				Name: "different_bands_2_plus_5_client",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceCompanionDUT,
					ClientDev: wificell.P2PDeviceDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(5180)},
					apOpts:    infraConfigLoBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP on 5Ghz band and is p2p GO on 2Ghz band .
				Name: "different_bands_5_plus_2_go",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceDUT,
					ClientDev: wificell.P2PDeviceCompanionDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
					apOpts:    infraConfigHiBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			}, {
				// Checks performance when DUT connects to AP on 5Ghz band and is a p2p client on 2Ghz band .
				Name: "different_bands_5_plus_2_client",
				Val: []p2pConcurrencyPerfTestcase{{
					GoDev:     wificell.P2PDeviceCompanionDUT,
					ClientDev: wificell.P2PDeviceDUT,
					p2pOpts:   []p2p.GroupOption{p2p.SetFreq(2462)},
					apOpts:    infraConfigHiBand,
					testType:  defaultTestTypes,
					powerSave: true,
				}},
			},
		},
	})
}

func P2PConcurrencyPerf(ctx context.Context, s *testing.State) {
	/*
		This test aims to test the concurrent performance of both WiFi Direct and Infra WiFi
		by using the following steps:
		1- Configure and connect the Infra WiFi link (main DUT: Client, Router)
		2- Configure and connect the P2P link (depending on test variant configuration)
		3- Check individual performance in sequence to get baseline:
		   1- The Infra WiFi connection (main DUT --> Router).
		   2- The P2P connection (DUT <--> Companion DUT).
		4- Run parallel performance tests on both connections.
		5- Compare results.
		6- Deconfigure the P2P link.
		7- Deconfigure the Infra WiFi link.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	ctx, restoreBgAndFg, err := tf.DUTWifiClient(wificell.DefaultDUT).TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	ctx, restoreBgAndFgPeer, err := tf.DUTWifiClient(wificell.PeerDUT1).TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFgPeer(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	testOnce := func(ctx context.Context, s *testing.State, tc p2pConcurrencyPerfTestcase) {
		s.Log("Configure INFRA connection")
		ap1, err := tf.ConfigureAP(ctx, tc.apOpts, nil)
		if err != nil {
			s.Fatal("Failed to configure ap, err: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap1); err != nil {
				s.Error("Failed to deconfig AP: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap1)
		defer cancel()

		if _, err := tf.ConnectWifiAP(ctx, ap1); err != nil {
			s.Fatal("Failed to connect to WiFi: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		s.Log("Configure P2P connection")
		if err := tf.P2PConfigureGO(ctx, wificell.P2PDeviceDUT, tc.p2pOpts...); err != nil {
			s.Fatal("Failed to configure the p2p group owner (GO): ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.P2PDeconfigureGO(ctx); err != nil {
				s.Error("Failed to deconfigure the p2p group owner (GO): ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDeconfigP2P(ctx)
		defer cancel()
		if err := tf.P2PConnect(ctx, wificell.P2PDeviceCompanionDUT); err != nil {
			s.Fatal("Failed to connect the p2p client to the p2p group owner (GO) network: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.P2PDisconnect(ctx); err != nil {
				s.Error("Failed to deconfigure the p2p client: ", err)
			}
		}(ctx)
		trafficTest := func(ctx context.Context, s *testing.State, testType perfmanager.TestType) {
			// Create an uncancellable copy of the context for cleanup.
			cleanupCtx := ctx

			// Individual tests.
			infraTest := func(ctx context.Context, perfTagPrefix string) (*iperf.Result, error) {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				ctx = testing.SetLogPrefix(ctx, "[Infra] ")
				return wifiutil.InfraPerf(ctx, cleanupCtx, tf, tf.DUTDevice(wificell.DefaultDUT), tc.powerSave, ap1, s.OutDir(),
					perfTagPrefix, testType, iperf.Version3)
			}

			p2pTest := func(ctx context.Context, perfTagPrefix string) (*iperf.Result, error) {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				ctx = testing.SetLogPrefix(ctx, "[ P2P ] ")

				p2pGO, _ := tf.P2PDevice(ctx, tc.GoDev)
				p2pClient, _ := tf.P2PDevice(ctx, tc.ClientDev)
				return wifiutil.P2PPerf(ctx, cleanupCtx, tf, p2pGO, p2pClient, s.OutDir(), perfTagPrefix, testType, iperf.Version3)
			}

			iperfCleanup := func(ctx context.Context) {
				// Make sure there's no iperf running from prior tests.
				wifiutil.Cleanup(ctx, []string{iperf.Version2, iperf.Version3}, []wificell.WiFiDevice{
					tf.DUTDevice(wificell.DefaultDUT),
					tf.DUTDevice(wificell.PeerDUT1),
					wifiutil.PerfRouter(tf)})
			}
			iperfCleanup(cleanupCtx)

			// Run test separately, so we have a data points for reference.
			s.Log("Starting tests in sequence to get reference data")
			initialInfraResult, err := infraTest(ctx, "individual_infra")
			if err != nil {
				s.Error("Failed to run initial Infra performance test: ", err)
				iperfCleanup(cleanupCtx)
				return
			}

			iperfCleanup(cleanupCtx)

			initialP2PResult, err := p2pTest(ctx, "individual_p2p")
			if err != nil {
				s.Error("Failed to run initial P2P performance test: ", err)
				iperfCleanup(cleanupCtx)
				return
			}

			iperfCleanup(cleanupCtx)

			// Combined throughput.
			s.Log("Start tests in parallel to measure performance drop")
			var infraResult, p2pResult *iperf.Result
			// Sometimes the perf tests that run in parallel don't finish in the same time. When one threads finish
			// early, we need to gracefully interrupt the other test thread so the results aren't distorted too much.
			// Create a cancellable context to facilitate that.
			c, mainCancel := context.WithCancel(ctx)

			// Create wait group for two goroutines.
			var wg sync.WaitGroup
			wg.Add(2)
			go func(ctx context.Context, wg *sync.WaitGroup) {
				defer wg.Done()
				infraResult, err = infraTest(ctx, "simultaneous_infra")

				if err == nil || !errors.Is(err, context.Canceled) {
					// Interrupt the other goroutine - measurements should be only performed when both are running.
					mainCancel()
				}
				if err != nil && !errors.Is(err, context.Canceled) {
					s.Error("Failed to run Infra performance test: ", err)
				}
			}(c, &wg)

			go func(ctx context.Context, wg *sync.WaitGroup) {
				defer wg.Done()
				p2pResult, err = p2pTest(ctx, "simultaneous_p2p")

				if err == nil || !errors.Is(err, context.Canceled) {
					// Interrupt the other goroutine - measurements should be only performed when both are running.
					mainCancel()
				}
				if err != nil && !errors.Is(err, context.Canceled) {
					s.Error("Failed to run P2P performance test: ", err)
				}
			}(c, &wg)

			s.Log("Waiting for perf tests to finish")
			wg.Wait()
			if !s.HasError() {
				infraThrDiff := 100 * infraResult.Throughput / initialInfraResult.Throughput
				p2pThrDiff := 100 * p2pResult.Throughput / initialP2PResult.Throughput
				s.Logf("Initial results: Infra:%v", initialInfraResult)
				s.Logf("Initial results:  P2P :%v", initialP2PResult)
				s.Logf("Final results(%v): Infra:%v (%3.2f%%)", testType, infraResult, infraThrDiff)
				s.Logf("Final results(%v):  P2P :%v (%3.2f%%)", testType, p2pResult, p2pThrDiff)
				s.Logf("Total throughput efficiency:%3.2f%%", infraThrDiff+p2pThrDiff)
				if infraThrDiff+p2pThrDiff < 80 {
					s.Error("Total throughput efficiency below 80%% threshold: ", infraThrDiff+p2pThrDiff)
				}
			}
			iperfCleanup(cleanupCtx)
		}
		for _, testType := range tc.testType {
			trafficTest(ctx, s, testType)
		}
	}

	testcases := s.Param().([]p2pConcurrencyPerfTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d: %s", i+1, len(testcases), tc.printableName), subtest)
	}
}
