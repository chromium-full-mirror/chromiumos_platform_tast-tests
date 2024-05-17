// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/wpacli"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

// scanPerfTestCase holds parameters of a ScanPerf test variant.
type scanPerfTestCase struct {
	// apOpts holds options to configure hostapd.
	apOpts []ap.Option
	// isPassive6GHzScan indicates WiFi RequestScan type, true = passive 6GHz-only scan, false = active full scan
	isPassive6GHzScan bool
}

// TODO(b/263890395): The following chipsets are known to fail the AVL. Remove
// a chip when its issue is fixed and its test results on the unstable test
// variants are healthy. For each chipset, tests are run twice:
// 1. Run in the regular suite with relaxed requirements so that we can make
// sure their performance will not deteriorate;
// 2. run in the wificell_unstable suite (corresponding test variants suffixed
// by "unstable") with regular requirements so that partners can verify their
// fix.
// For example, chipsets listed below will run once in wifi.ScanPerf.dtim1 with
// relaxed requirements and another time in wifi.ScanPerf.dtim1unstable with
// regular requirements.
var deviceWithUnstableScan = []wlan.DeviceID{
	wlan.QualcommWCN6750,
	wlan.QualcommWCN6855,
	wlan.MediaTekMT7921PCIE,
	wlan.MediaTekMT7921SDIO,
	wlan.Realtek8852CPCIE,
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ScanPerf,
		Desc: "Measure BSS scan performance in various setup",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_perf"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wificell.BluetoothServiceName,
		},
		Vars:         []string{"router", "pcap"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassPerf, tdreq.WiFiProcPassPerfBeforeUpdates},
		Params: []testing.Param{
			{
				// Default case, DTIM = 2
				// See https://source.corp.google.com/chromeos_public/src/third_party/wpa_supplicant-cros/next/src/ap/ap_config.c;rcl=20a522b9ebe52bac34cc4ecfc1a9722cc1e77cdc;l=88
				// Since crrev.com/c/3996676, averages of full scan times are recorded in stead of one full scan.
				Val: scanPerfTestCase{
					isPassive6GHzScan: false,
				},
			},
			{
				Name: "passive6ghz",
				Val: scanPerfTestCase{
					isPassive6GHzScan: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
			},
			{
				// This variant runs on unstable chipsets with default parameters.
				Name: "unstable",
				Val: scanPerfTestCase{
					isPassive6GHzScan: false,
				},
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiDevice(deviceWithUnstableScan...)),
			},
			{
				Name: "dtim1",
				Val: scanPerfTestCase{
					apOpts:            []ap.Option{ap.DTIMPeriod(1)},
					isPassive6GHzScan: false,
				},
			},
			{
				Name: "dtim1unstable",
				Val: scanPerfTestCase{
					apOpts:            []ap.Option{ap.DTIMPeriod(1)},
					isPassive6GHzScan: false,
				},
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiDevice(deviceWithUnstableScan...)),
			},
		},
	})
}

func ScanPerf(ctx context.Context, s *testing.State) {
	/*
		This test measures WiFi scan time with established network connection (background scan)
		or without (foreground scan) and compares with thresholds to indicate pass or not.
		Full (wildcard scan on all channels) scan times are obtained as avg from tests with `scanTimes` times.
		Thresholds are applied to each single full scan test.
		Here are the steps:
		1- Configures the AP (e.g. specifies DTIM value).
		2- Performs full foreground scan multiple times.
		3- Full background scan:
		3-1- Connect DUT to AP
		3-2- Performs multiple scans.
		4- Deconfigures from defer() stack.
	*/

	const (
		// Repeated scan times to obtain averages.
		scanTimes = 5

		// Upper bounds for different scan methods.
		fgFullScanTimeout = 15 * time.Second
		bgFullScanTimeout = 20 * time.Second
		pollTimeout       = 15 * time.Second

		// Thresholds for scan tests.
		fgFullScanThreshold = 4 * time.Second
		bgFullScanThreshold = 8 * time.Second
		// Thresholds for passive scan tests.
		fgPassiveScanThreshold = 7 * time.Second
		bgPassiveScanThreshold = 13 * time.Second
	)

	// TODO(b/260276685): Shared fixture among test variants causes a longer 1st bg when dtim config is different from the last subtest.
	// Create a new test fixture for each test variant. Use shared |wificellFixt| when fixed.
	tfOps := wificell.NewTFOptionsBuilder()
	tfOps.DutTarget(s.DUT(), s.RPCHint())
	if router, ok := s.Var("router"); ok && router != "" {
		tfOps.PrimaryRouterTargets(router)
	}
	if pcap, ok := s.Var("pcap"); ok && pcap != "" {
		tfOps.PcapRouterTarget(pcap)
	}
	tfOps.EnablePacketCapture(true)
	// TODO(b/279663413): Tests should not manually initialize the wifi test fixture class.
	tf, err := wificell.NewTestFixture(ctx, ctx, tfOps.Build())
	if err != nil {
		s.Fatal("Failed to set up test fixture: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.Close(ctx); err != nil {
			s.Error("Failed to properly take down test fixture: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForClose(ctx)
	defer cancel()

	initialRegDomain, err := tf.InitializeRegdomainUS(ctx)
	if err != nil {
		s.Fatal("Failed to initialize the regulatory domain: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.ResetRegdomain(ctx, initialRegDomain); err != nil {
			s.Error("Failed to reset the regulatory domain: ", err)
		}
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
	defer cancel()

	r, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect rpc: ", err)
	}
	defer r.Close(ctx)

	options := wificell.DefaultOpenNetworkAPOptions()
	tc := s.Param().(scanPerfTestCase)
	options = append(options, tc.apOpts...)

	var requestScanType string
	if tc.isPassive6GHzScan {
		requestScanType = shillconst.WiFiRequestScanTypePassive
	} else {
		requestScanType = shillconst.WiFiRequestScanTypeActive
	}

	apIface, err := tf.ConfigureAP(ctx, options, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, apIface); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, apIface)
	defer cancel()
	s.Log("AP setup done")

	ssid := apIface.Config().SSID

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	wpaMonitor, stop, ctx, err := tf.StartWPAMonitor(ctx, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("Failed to start wpa monitor")
	}
	defer stop()

	runner := wpacli.NewRunner(&cmd.RemoteCmdRunner{Host: s.DUT().Conn()})

	originalRequestScanType, err := tf.WifiClient().GetRequestScanTypeProperty(ctx)
	if err != nil {
		s.Fatal("Failed to get WiFi RequestScan type: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, originalRequestScanType); err != nil {
			s.Errorf("Failed to reset WiFi RequestScan type to %s: %v", originalRequestScanType, err)
		}
		s.Log("Reset WiFi RequestScan type to ", originalRequestScanType)
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
	defer cancel()

	logDuration := func(label string, duration time.Duration) {
		pv.Set(perf.Metric{
			Name:      label,
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
		}, duration.Seconds())
		s.Logf("%s: %s", label, duration)
	}

	// pollTimedScan polls RequestScan and returns scan duration.
	// Each scan takes at most scanTimeout, and the polling takes at most pollTimeout.
	pollTimedScan := func(ctx context.Context, scanTimeout, pollTimeout time.Duration, ssid string) (time.Duration, error) {
		var scanTime time.Duration
		var startTime time.Time
		if pollTimeout < scanTimeout {
			pollTimeout = scanTimeout
		}

		ctx, cancel := context.WithTimeout(ctx, pollTimeout)
		defer cancel()

		err := testing.Poll(ctx, func(ctx context.Context) error {
			wpaMonitor.ClearEvents(ctx)

			if err := tf.WifiClient().RequestScan(ctx); err != nil {
				return errors.Wrap(err, "failed to request scan")
			}
			if err := func(ctx context.Context) error {
				scanStartCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
				defer cancel()
				for {
					event, err := wpaMonitor.WaitForEvent(scanStartCtx)
					if err != nil {
						return errors.Wrap(err, "failed to wait for ScanStarted event")
					}
					if event == nil { // timeout
						return errors.New("waiting for ScanStarted event timeout")
					}
					if _, ok := event.(*wpacli.ScanStartedEvent); ok {
						startTime = time.Now()
						return nil
					}
				}
			}(ctx); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: pollTimeout})
		if err != nil {
			return 0, err
		}

		for {
			event, err := wpaMonitor.WaitForEvent(ctx)
			if err != nil {
				return 0, errors.Wrap(err, "failed to wait for ScanResults event")
			}
			if event == nil { // timeout
				return 0, errors.New("waiting for ScanResults event timeout")
			}
			if _, ok := event.(*wpacli.ScanResultsEvent); ok {
				scanTime = time.Since(startTime)
				break
			}
		}
		// If the DUT performs passive 6GHz-only scan, it doesn't scan legacy channels and thus cannot discover the AP on 5 GHz band
		if requestScanType != shillconst.WiFiRequestScanTypePassive {
			if err := runner.CheckScanResults(ctx, ssid); err != nil {
				return 0, errors.Wrap(err, "failed to discover AP")
			}
		}

		return scanTime, nil
	}

	// Foreground full scan.
	count := 0
	var maxScanTime time.Duration
	threshold := fgFullScanThreshold
	if requestScanType == shillconst.WiFiRequestScanTypePassive {
		threshold = fgPassiveScanThreshold
	}
	if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, requestScanType); err != nil {
		s.Fatalf("Failed to set WiFi RequestScan type to %s: %v", requestScanType, err)
	}
	s.Log("Set WiFi RequestScan type to ", requestScanType)
	for i := 1; i <= scanTimes; i++ {
		if duration, err := pollTimedScan(ctx, fgFullScanTimeout, pollTimeout, ssid); err != nil {
			s.Error("Failed to perform full channel scan: ", err)
		} else {
			if duration > threshold {
				s.Errorf("Foreground scan #(%d/%d) duration: %s. Exceed threshold: %s", i, scanTimes, duration, threshold)
			} else {
				s.Logf("Foreground scan #(%d/%d) duration: %s", i, scanTimes, duration)
			}
			if duration > maxScanTime {
				maxScanTime = duration
			}
			count++
		}
	}
	if count == 0 {
		s.Error("Failed to perform all full channel scans in foreground scan test")
	} else {
		// Reports max value to crosbolt so that data from different vendors can
		// be compared. Some vendors optimize 3-4 scans (shorter) in every 5
		// scans so report the longest result (see discussion in b/284533163).
		s.Logf("Max foreground scan duration: %s", maxScanTime)
		logDuration("scan_time_foreground_full", maxScanTime)
	}

	// Background full scan.
	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	// If the WiFi RequestScan type is passive, switch to active scan so that the DUT can discover and connect to the AP
	if requestScanType == shillconst.WiFiRequestScanTypePassive {
		if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, shillconst.WiFiRequestScanTypeActive); err != nil {
			s.Fatalf("Failed to set WiFi RequestScan type to %s: %v", shillconst.WiFiRequestScanTypeActive, err)
		}
		s.Log("Set WiFi RequestScan type to active to discover and connect to the AP")
	}

	// DUT connecting to the AP.
	if _, err := tf.ConnectWifiAP(ctx, apIface); err != nil {
		s.Fatal("DUT: failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	s.Log("Connected")

	count = 0
	maxScanTime = 0
	threshold = bgFullScanThreshold
	if requestScanType == shillconst.WiFiRequestScanTypePassive {
		threshold = bgPassiveScanThreshold
	}
	if requestScanType == shillconst.WiFiRequestScanTypePassive {
		if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, requestScanType); err != nil {
			s.Fatalf("Failed to set WiFi RequestScan type to %s: %v", requestScanType, err)
		}
		s.Log("Set WiFi RequestScan type to ", requestScanType)
	}
	for i := 1; i <= scanTimes; i++ {
		if duration, err := pollTimedScan(ctx, bgFullScanTimeout, pollTimeout, ssid); err != nil {
			s.Error("Failed to perform full channel scan: ", err)
		} else {
			if duration > threshold {
				s.Errorf("Background scan #(%d/%d) duration: %s. Exceed threshold: %s", i, scanTimes, duration, threshold)
			} else {
				s.Logf("Background scan #(%d/%d) duration: %s", i, scanTimes, duration)
			}
			if duration > maxScanTime {
				maxScanTime = duration
			}
			count++
		}
	}
	if count == 0 {
		s.Error("Failed to perform all full channel scans in background scan test")
	} else {
		// Reports max value to crosbolt so that data from different vendors can
		// be compared. Some vendors optimize 3-4 scans (shorter) in every 5
		// scans so report the longest result (see discussion in b/284533163).
		s.Logf("Max background scan duration: %s", maxScanTime)
		logDuration("scan_time_background_full", maxScanTime)
	}
}
