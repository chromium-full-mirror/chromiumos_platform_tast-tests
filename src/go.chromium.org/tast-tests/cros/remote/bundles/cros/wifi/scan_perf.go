// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/network/wpacli"
	"go.chromium.org/tast-tests/cros/common/perf"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
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
	// useRelaxedThreshold indicates whether to allow extra time for WiFi scan.
	useRelaxedThreshold bool
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
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wificell.BluetoothServiceName,
		},
		Vars:         []string{"router"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassPerf, tdreq.WiFiProcPassPerfBeforeUpdates},
		Params: []testing.Param{
			{
				// Default case, DTIM = 2
				// See https://source.corp.google.com/chromeos_public/src/third_party/wpa_supplicant-cros/next/src/ap/ap_config.c;rcl=20a522b9ebe52bac34cc4ecfc1a9722cc1e77cdc;l=88
				// Since crrev.com/c/3996676, averages of full scan times are recorded in stead of one full scan.
				Val: scanPerfTestCase{
					useRelaxedThreshold: true,
				},
			},
			{
				// This variant runs on unstable chipsets with default parameters.
				Name: "unstable",
				Val: scanPerfTestCase{
					useRelaxedThreshold: false,
				},
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiDevice(deviceWithUnstableScan...)),
			},
			{
				Name: "dtim1",
				Val: scanPerfTestCase{
					apOpts:              []ap.Option{ap.DTIMPeriod(1)},
					useRelaxedThreshold: true,
				},
			},
			{
				Name: "dtim1unstable",
				Val: scanPerfTestCase{
					apOpts:              []ap.Option{ap.DTIMPeriod(1)},
					useRelaxedThreshold: false,
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
		fgFullScanTimeout = 10 * time.Second
		bgFullScanTimeout = 15 * time.Second
		pollTimeout       = 15 * time.Second

		// Thresholds for scan tests.
		fgFullScanThreshold        = 4 * time.Second
		bgFullScanThreshold        = 7 * time.Second
		bgFullScanThresholdRelaxed = 9 * time.Second
		// TODO(b/256486257): Move these 6E requirements to new test variants when new AVL requirements are settled.
		fgFullScanThresholdWiFi6ERelaxed = 15 * time.Second
		bgFullScanThresholdWiFi6ERelaxed = 15 * time.Second
	)

	// TODO(b/253096914): The following chipsets are known to have slower bg scan times.
	// Use relaxed threshold until the bug has been solved.
	bgRelaxedChipsets := map[wlan.DeviceID]struct{}{
		wlan.MediaTekMT7921PCIE: {},
		wlan.MediaTekMT7921SDIO: {},
	}

	// TODO(b/256486257): We lack data for WiFi6E models so threshold is not determined yet.
	// Temporarily set to sufficiently long values to make those tests always pass for those fail to reach regular thresholds.
	wifi6eRelaxedChipsets := map[wlan.DeviceID]struct{}{
		wlan.QualcommWCN6855:  {},
		wlan.QualcommWCN6750:  {},
		wlan.Realtek8852CPCIE: {},
	}

	// TODO(b/260276685): Shared fixture among test variants causes a longer 1st bg when dtim config is different from the last subtest.
	// Create a new test fixture for each test variant. Use shared |wificellFixt| when fixed.
	tfOps := wificell.NewTFOptionsBuilder()
	tfOps.DutTarget(s.DUT(), s.RPCHint())
	if router, ok := s.Var("router"); ok && router != "" {
		tfOps.PrimaryRouterTargets(router)
	}
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

	r, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect rpc: ", err)
	}
	defer r.Close(ctx)

	client := wifi.NewShillServiceClient(r.Conn)

	// Get the information of the WLAN device.
	devInfo, err := client.GetDeviceInfo(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed obtaining WLAN device information through rpc: ", err)
	}
	devID := wlan.DeviceID(devInfo.Id)

	options := wificell.DefaultOpenNetworkAPOptions()
	tc := s.Param().(scanPerfTestCase)
	options = append(options, tc.apOpts...)

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

		if err := runner.CheckScanResults(ctx, ssid); err != nil {
			return 0, errors.Wrap(err, "failed to discover AP")
		}
		return scanTime, nil
	}

	// Foreground full scan.
	count := 0
	var sum time.Duration
	threshold := fgFullScanThreshold
	if tc.useRelaxedThreshold {
		if _, ok := wifi6eRelaxedChipsets[devID]; ok {
			threshold = fgFullScanThresholdWiFi6ERelaxed
			s.Logf("There is a known issue (b/256486257) for this WiFi6E chip (%s), use a sufficiently long threshold and this test always passes", devInfo.Name)
		}
	}
	for i := 1; i <= scanTimes; i++ {
		if duration, err := pollTimedScan(ctx, fgFullScanTimeout, pollTimeout, ssid); err != nil {
			s.Error("Failed to perform full channel scan: ", err)
		} else {
			if duration > threshold {
				s.Errorf("Foreground scan #(%d/%d) duration: %s. Exceed threshold: %s", i, scanTimes, duration, threshold)
			} else {
				s.Logf("Foreground scan #(%d/%d) duration: %s", i, scanTimes, duration)
			}
			sum += duration
			count++
		}
	}
	if count == 0 {
		s.Error("Failed to perform all full channel scans in foreground scan test")
	} else {
		avg := time.Duration(int64(sum) / int64(count))
		s.Logf("Foreground scan average duration: %s", avg)
		logDuration("scan_time_foreground_full", avg)
	}

	// Background full scan.
	ctx, restoreBg, err := tf.WifiClient().TurnOffBgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background scan: ", err)
	}
	defer func() {
		if err := restoreBg(); err != nil {
			s.Error("Failed to restore the background scan config: ", err)
		}
	}()

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
	sum = 0
	threshold = bgFullScanThreshold
	if tc.useRelaxedThreshold {
		if _, ok := bgRelaxedChipsets[devID]; ok {
			threshold = bgFullScanThresholdRelaxed
			s.Logf("There is a known issue (b/253096914) for this WiFi chip (%s), use a relaxed threshold: %s", devInfo.Name, threshold)
		} else if _, ok := wifi6eRelaxedChipsets[devID]; ok {
			threshold = bgFullScanThresholdWiFi6ERelaxed
			s.Logf("There is a known issue (b/256486257) for this WiFi6E chip (%s), use a sufficiently long threshold and this test always passes", devInfo.Name)
		}
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
			sum += duration
			count++
		}
	}
	if count == 0 {
		s.Error("Failed to perform all full channel scans in background scan test")
	} else {
		avg := time.Duration(int64(sum) / int64(count))
		s.Logf("Background scan average duration: %s", avg)
		logDuration("scan_time_background_full", avg)
	}
}
