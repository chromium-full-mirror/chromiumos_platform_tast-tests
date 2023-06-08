// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// sapOnOffRounds can be changed to manually extend the test for measurements in a longer period.
const sapOnOffRounds = 25

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPOnOffStress,
		Desc: "Verifies that SAP still works normally and there are no crash or resource leaks after multiple on/off cycles",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"jck@semihalf.com",                // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap", "wificell_cross_device_unstable"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT),
		Timeout:      time.Minute + 8*sapOnOffRounds*20*time.Second, // Default: 18 minutes, typically test runs in half that time.
		ServiceDeps:  []string{wificell.ShillServiceName},
		SoftwareDeps: []string{"wpa3_sae"},
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Params: []testing.Param{
			{
				Val: []wifiutil.SAPOnOffStressTestcase{
					// No encryption in low band and high band.
					{
						PrintableName: "open_2_4G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
						UseWpaCliAPI:  true,
					}, {
						PrintableName: "open_5G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
						UseWpaCliAPI:  true,
					},
					// WPA2 encryption in low band and high band.
					{
						PrintableName: "wpa2_2_4G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA2)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						PrintableName: "wpa2_5G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA2)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
						),
					},
					// WPA3 encryption in low band and high band.
					{
						PrintableName: "wpa3_2_4G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA3)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						PrintableName: "wpa3_5G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA3)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					},
					// WPA3 transitional encryption in low band and high band.
					{
						PrintableName: "wpa3mixed_2_4G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModeMixedWPA3)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						PrintableName: "wpa3mixed_5G",
						TetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModeMixedWPA3)},
						SecConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}},
			},
		},
	})
}

func SAPOnOffStress(ctx context.Context, s *testing.State) {
	/*
		This test checks the soft AP resource utilization on associate/disassociate:
		1- Disable the station interface.
		2- Measure initial memory/open fd values.
		3- In loop (1..N)
		3a -Configure the main DUT as a soft AP.
		3b- Configures the Companion DUT as a STA.
		3c- Connects the the STA to the soft AP.
		3d- Verify the connection by running ping from the STA.
		3e- Deconfigure the STA.
		3f- Deconfigure the soft AP.
		3g- Record intermediate memory/open fd values.
		3h- Check that tracked processes PIDs haven't changed.
		4- Measure final memory/open fd values.
		5- Make sure memory in use did not rise substantially and number of FDs is stable.
		6- Re-enable the station interface.
	*/
	// Thresholds for acceptable changes of various counters (in %).
	var thresholds = wifiutil.ResourceThreshold{"vsz": 5, "fd": 0}
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

	testcases := s.Param().([]wifiutil.SAPOnOffStressTestcase)
	for i, tc := range testcases {
		s.Run(ctx, fmt.Sprintf("Testcase #%d/%d: %s", i+1, len(testcases), tc.PrintableName),
			func(ctx context.Context, s *testing.State) {
				// The main test runs here.
				if err := wifiutil.SAPOnOffStressTest(ctx, s, tf, tc, sapOnOffRounds,
					thresholds, processes, pv); err != nil {
					s.Fatal("SAPOnOffStressTest failed: ", err)
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
