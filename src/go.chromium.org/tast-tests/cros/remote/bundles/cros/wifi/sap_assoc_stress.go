// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/tethering"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type sapAssocStressTestcase struct {
	printableName string
	tetheringOpts []tethering.Option
	secConfFac    security.ConfigFactory
	useWpaCliAPI  bool // Use wpa_cli API to setup tethering.
}

// sapAssocRounds can be changed to manually extend the test for measurements in a longer period.
const sapAssocRounds = 25

func init() {
	testing.AddTest(&testing.Test{
		Func: SAPAssocStress,
		Desc: "Verifies that SAP still works normally and there are no crash or resource leaks after multiple STA connections",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates},
		Timeout:      time.Minute + 8*sapAssocRounds*10*time.Second, // Default: 10 minutes, typically test runs in half that time.
		ServiceDeps:  []string{wificell.ShillServiceName},
		SoftwareDeps: []string{"wpa3_sae"},
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Params: []testing.Param{
			{
				Val: []sapAssocStressTestcase{
					// No encryption in low band and high band.
					{
						printableName: "open_2_4G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true)},
						useWpaCliAPI:  true,
					}, {
						printableName: "open_5G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true)},
						useWpaCliAPI:  true,
					},
					// WPA2 encryption in low band and high band.
					{
						printableName: "wpa2_2_4G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA2)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						printableName: "wpa2_5G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA2)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
						),
					},
					// WPA3 encryption in low band and high band.
					{
						printableName: "wpa3_2_4G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA3)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						printableName: "wpa3_5G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModePureWPA3)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					},
					// WPA3 transitional encryption in low band and high band.
					{
						printableName: "wpa3mixed_2_4G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band2p4g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModeMixedWPA3)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					}, {
						printableName: "wpa3mixed_5G",
						tetheringOpts: []tethering.Option{tethering.Band(tethering.Band5g), tethering.NoUplink(true),
							tethering.SecMode(wpa.ModeMixedWPA3)},
						secConfFac: wpa.NewConfigFactory(
							"chromeos", wpa.Mode(wpa.ModeMixedWPA3), wpa.Ciphers2(wpa.CipherCCMP),
						),
					},
				},
			},
		},
	})
}

func SAPAssocStress(ctx context.Context, s *testing.State) {
	/*
		This test checks the soft AP resource utilization on associate/disassociate:
		1- Disable the station interface.
		2- Configures the main DUT as a soft AP.
		3- Measure initial memory/open fd values.
		4- In loop (1..N)
		4a- Configures the Companion DUT as a STA.
		4b- Connects the the STA to the soft AP.
		4c- Verify the connection by running ping from the STA.
		4d- Deconfigure the STA.
		4e- Record intermediate memory/open fd values.
		4f- Check that tracked processes PIDs haven't changed.
		5- Measure final memory/open fd values.
		6- Make sure memory in use did not rise substantially and number of FDs is stable.
		7- Deconfigure the soft AP.
		8- Re-enable the station interface.
	*/
	// Thresholds for acceptable changes of various counters (vsz in %, fd in absolute number).
	var thresholds = wifiutil.ResourceThreshold{"vsz": 5, "fd": -10}
	const processes = "shill,wpa_supplicant,patchpaneld"

	tf := s.FixtValue().(*wificell.TestFixture)
	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test requires at least 2 DUTs to be declared. Only have ", tf.NumberOfDUTs())
	}

	testOnce := func(ctx context.Context, s *testing.State, tc sapAssocStressTestcase) {
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

		resInfo, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
		if err != nil {
			s.Fatal("Failed to get resource info: ", err)
		}
		resInfos := []wifiutil.ResourceInfo{resInfo}

		// Make sure output is recorded even in case of error, this might be the reason of the issue.
		defer func(ctx context.Context) {
			if err := wifiutil.ReportResources(ctx, resInfos, s.OutDir(), fmt.Sprintf("%s.tsv", "sap_assoc_"+tc.printableName)); err != nil {
				s.Error("Failed to write resources report: ", err)
			}
		}(ctx)
		// Reserve enough time for the report function to save data.
		ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		// We're running in a simple loop instead of s.Run() on purpose, we want to bail out on the first error.
		for i := 0; i < sapAssocRounds; i++ {
			testing.ContextLogf(ctx, "Connection round #%v starts", i+1)
			err = wifiutil.SAPAssocStressRound(ctx, tf, tetheringConf)
			if err != nil {
				s.Fatal("Failure during stress round: ", err)
			}
			resInfo, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
			if err != nil {
				s.Fatal("Failed to get resource info: ", err)
			}
			// Check if pid of processes changed.
			if err := wifiutil.ValidatePids(resInfos[0], resInfo); err != nil {
				s.Error("Error while validating PIDs, err: ", err)
			}
			testing.ContextLogf(ctx, "Connection round #%d finished: %s", i+1, resInfo.String())
			resInfos = append(resInfos, resInfo)
		}
		testing.ContextLog(ctx, "Start: ", resInfos[0].String())
		testing.ContextLog(ctx, "End:   ", resInfos[len(resInfos)-1].String())
		if err := wifiutil.ValidateResourceInfo(ctx, resInfos[0], resInfos[len(resInfos)-1], thresholds); err != nil {
			s.Error("Resource validation failed, err: ", err)
		}
	}
	// Global resource check, over the whole test.
	resStart, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		s.Fatal("Failed to get resource info: ", err)
	}

	testcases := s.Param().([]sapAssocStressTestcase)
	for i, tc := range testcases {
		subtest := func(ctx context.Context, s *testing.State) {
			testOnce(ctx, s, tc)
		}
		s.Run(ctx, fmt.Sprintf("Testcase #%d: %s", i, tc.printableName), subtest)
	}

	resEnd, err := wifiutil.GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		s.Fatal("Failed to get resource info: ", err)
	}
	testing.ContextLog(ctx, "Total resource validation, start value: ", resStart.String())
	testing.ContextLog(ctx, "Total resource validation, end value: ", resEnd.String())
	if err := wifiutil.ValidateResourceInfo(ctx, resStart, resEnd, thresholds); err != nil {
		s.Fatal("Total resource validation failed, err: ", err)
	}

	s.Log("Tearing down")
}
