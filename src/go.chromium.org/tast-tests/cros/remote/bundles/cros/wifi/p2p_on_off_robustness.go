// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// p2pOnOffRounds can be changed to manually extend the test for measurements in a longer period.
const p2pOnOffDefaultRounds = 25
const p2pOnOffExtendedRounds = 500

func init() {
	testing.AddTest(&testing.Test{
		Func: P2POnOffRobustness,
		Desc: "Verifies that P2P still works normally and there are no crash or resource leaks after multiple on/off cycles",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"jsiuda@google.com",               // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		ServiceDeps:  []string{wificell.ShillServiceName},
		HardwareDepsForAll: map[string]hwdep.Deps{
			"":    hwdep.D(hwdep.WifiP2P()),
			"cd1": hwdep.D(hwdep.WifiP2P()),
		},
		Params: []testing.Param{
			{
				ExtraAttr: []string{"wificell_cross_device_p2p", "wificell_cross_device_unstable"},
				// Default timeout: 11 minutes, typically test runs in half of that time.
				Timeout: time.Minute + 2*p2pOnOffDefaultRounds*12*time.Second,
				Val: []wifiutil.P2POnOffRobustnessTestcase{
					{
						PrintableName: "2_4G",
						Opts:          []p2p.GroupOption{p2p.SetFreq(2462)},
						Rounds:        p2pOnOffDefaultRounds,
					}, {
						PrintableName: "5G",
						Opts:          []p2p.GroupOption{p2p.SetFreq(5180)},
						Rounds:        p2pOnOffDefaultRounds,
					}},
			},
			{
				Name: "extended",
				// No "wificell_cross_device_p2p" ExtraAttr effectively disables test from scheduling,
				// while keeping the option to run the test variant manually.
				// Default timeout: 201 minutes, typically test runs in half of that time.
				Timeout: time.Minute + 2*p2pOnOffExtendedRounds*12*time.Second,
				Val: []wifiutil.P2POnOffRobustnessTestcase{
					{
						PrintableName: "extended_2_4G",
						Opts:          []p2p.GroupOption{p2p.SetFreq(2462)},
						Rounds:        p2pOnOffExtendedRounds,
					}, {
						PrintableName: "extended_5G",
						Opts:          []p2p.GroupOption{p2p.SetFreq(5180)},
						Rounds:        p2pOnOffExtendedRounds,
					}},
			},
		},
	})
}

func P2POnOffRobustness(ctx context.Context, s *testing.State) {
	/*
		This test checks the P2P resource utilization on P2P on/off:
		1- Measure initial memory/open fd values.
		2- In loop (1..N)
		2a -Configure the main DUT as a P2P GO.
		2b- Connects the Companion DUT as a P2P Client.
		2c- Verify the connection by running ping from GO.
		2d- Disconnect the P2P Client.
		2e- Deconfigure the P2P Group.
		2f- Record intermediate memory/open fd values.
		2g- Check that tracked processes PIDs haven't changed.
		3- Measure final memory/open fd values.
		4- Make sure memory in use did not rise substantially and number of FDs is stable.
	*/
	// Thresholds for acceptable changes of various counters (vsz in %, fz in absolute number).
	var thresholds = wifiutil.ResourceThreshold{"vsz": 5, "fd": -10}
	const processes = "shill,wpa_supplicant,patchpaneld"

	tf := s.FixtValue().(*wificell.TestFixture)
	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test requires at least 2 DUTs to be declared. Only have ", tf.NumberOfDUTs())
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Log("Failed to save perf data: ", err)
		}
	}()
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Second)
	defer cancel()

	// Global resource check, over the whole test.
	resStart, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		s.Fatal("Failed to get resource info: ", err)
	}

	testcases := s.Param().([]wifiutil.P2POnOffRobustnessTestcase)
	for i, tc := range testcases {
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d: %s", i+1, len(testcases), tc.PrintableName),
			func(ctx context.Context, s *testing.State) {
				// The main test runs here.
				if err := wifiutil.P2POnOffRobustnessTest(ctx, s, tf, tc, tc.Rounds,
					thresholds, processes, pv); err != nil {
					s.Fatal("P2POnOffRobustness failed: ", err)
				}
			})
	}

	resEnd, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		s.Fatal("Failed to get resource info: ", err)
	}
	if err := wifiutil.ValidateResourceInfo(ctx, resStart, resEnd, thresholds); err != nil {
		s.Error("Total resource validation failed: ", err)
	}

	s.Log("Tearing down")
}
