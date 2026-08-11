// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/tracing"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Give a 20 second buffer to make sure we can attempt to roam back to
	// the original AP before the retry delay is over.
	bssTMReassocDelay  = wifiutil.BSSTMRequestTimeout + 20*time.Second
	bssTMReassocBuffer = 5 * time.Second
)

type bssTMReqTestCase struct {
	// requestParams defines the parameters for the BSS Transition Management Request.
	requestParams hostapd.BSSTMReqParams
	// secConfFac1 is the security configuration factory for the first AP.
	secConfFac1 security.ConfigFactory
	// secConfFac2 is the security configuration factory for the second AP.
	secConfFac2 security.ConfigFactory
	// pmfRequiredAP1 indicates whether AP 1 should enable the Hostapd config |PMFRequired|.
	pmfRequiredAP1 bool
	// pmfRequiredAP2 indicates whether AP 2 should enable the Hostapd config |PMFRequired|.
	pmfRequiredAP2 bool
	// ap1Opts is the list of options to be used for the first AP.
	ap1Opts []hostapd.Option
	// ap2Opts is the list of options to be used for the second AP.
	ap2Opts []hostapd.Option
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BSSTMRequest,
		Desc: "Tests the DUTs response to a BSS Transition Management Request",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "group:release-health", "release-health_wifi"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName, wificell.TraceCmdService},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiGenSupportMBO, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Val: bssTMReqTestCase{},
			},
			{
				Name:              "disassoc_imminent",
				ExtraSoftwareDeps: []string{"mbo"},
				Val: bssTMReqTestCase{
					requestParams: hostapd.BSSTMReqParams{
						DisassocImminent: true,
						DisassocTimer:    wifiutil.BSSTMRequestTimeout,
						ReassocDelay:     bssTMReassocDelay,
					},
				},
			},
			{
				Name: "bss_term",
				Val: bssTMReqTestCase{
					requestParams: hostapd.BSSTMReqParams{
						BSSTerm: 1 * time.Minute,
					},
				},
			},
			{
				// Verifies that DUT can roam from a BSS with PSK key management to a BSS with SAE key management and back.
				Name: "psk_to_sae",
				Val: bssTMReqTestCase{
					secConfFac1:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					secConfFac2:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					pmfRequiredAP2: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				// Verifies that DUT can roam from a BSS with SAE key management to a BSS with PSK key management and back.
				Name: "sae_to_psk",
				Val: bssTMReqTestCase{
					secConfFac1:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					secConfFac2:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					pmfRequiredAP1: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				// Verifies that DUT can roam from a BSS with SAE key management to a BSS with PSK key management and back.
				Name: "psk_to_sae_6ghz",
				Val: bssTMReqTestCase{
					secConfFac1:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					secConfFac2:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					pmfRequiredAP1: true,
					ap1Opts:        []hostapd.Option{hostapd.Mode(hostapd.Mode80211axMixed), hostapd.Channel(40), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.SpectrumManagement()},
					ap2Opts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.Channel(21), hostapd.HTCaps(hostapd.HTCapHT20),
						hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.Is6GHz(), hostapd.PMF(hostapd.PMFRequired)},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
				ExtraTestBedDeps:  tbdep.WifiRouterFeatures(labapi.WifiRouterFeature_WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
		},
	})
}

/*
	This test vefifies a device's behavior when it receives a BSS Transition
	Management Request. Specifically, it is expected that a device should move
	away from it's current BSS when it receives a BSSTM Request from that BSS.
	This is verified by first, monitoring the Shill RoamState property and
	asserting that the device roams away from the current BSS, and second,
	conducting a ping test to ensure connectivity at the resultant BSS.

	Here are the full steps this test takes:
	1- Disallow roaming from locally requested scans and turn off background
		scans to ensure that the only roams that happen are those triggered by the
		BSSTM Request
	2- Set up an AP "AP1" using security configs from |secConfFac1| if present.
	3- Connect to AP1.
	4- Set up another AP "AP2" using security configs from |secConfFac2| if present.
	5- Add the BSSID at AP1 into the DUT's ignorelist.
	6- Send a BSSTM request from AP1.
	7- Assert that the Shill property RoamState transitions from configuration
		-> ready -> idle, which indicates a roam has occurred.
	8- Assert that the BSSID property in Shill is equal to the BSSID from AP2.
	9- Conduct a ping test to ensure we are connected to AP2.
	10- If "disassoc_imminent" is enabled, send another BSSTM Request from AP2,
		but this time assert that the roam fails due to the reassociation
		delay.
	11- Send a BSSTM Request from AP2 without dissasoc imminent.
	12- Assert that the Shill property RoamState transitions from configuration
		-> ready -> idle, which indicates a roam has occurred.
	13- Assert that the BSSID property in Shill is equal to the BSSID from AP1.
	14- Conduct a ping test to ensure we are connected to AP1.
	15- Clean up state and revert the steps from (1).
*/

func BSSTMRequest(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	// TODO(b/377913176): Remove CollectIntelFirmwareDumpOnErr.
	tf.CollectIntelFirmwareDumpOnError = true

	runTest := func(ctx context.Context, s *testing.State, waitForScan bool) {
		// TODO(b/377913176): Remove the intel_wifi tracing.
		devInfo, err := tf.WifiClient().GetDeviceInfo(ctx, &empty.Empty{})
		if err != nil {
			s.Log("Failed to get the WiFi device information: ", err)
		}
		modelName, err := tf.DUTConn(wificell.DefaultDUT).CommandContext(ctx, "cros_config", "/", "name").Output()
		if err != nil {
			s.Log("Failed to get the model name: ", err)
		}
		boardName := string(bytes.TrimSuffix(modelName, []byte{'\n'}))
		if devInfo.Vendor == wificell.IntelVendorNum && (boardName == "riven" || boardName == "karis") {
			isTracingInstanceCreated := true
			isTracingInstanceStarted := true
			traceCmdEvents := "iwlwifi_data,iwlwifi_dev_tx,iwlwifi_dev_rx,iwlwifi,iwlwifi_msg,iwlwifi_ucode"
			instanceName := "intel_wifi"
			cl := tf.RPC()
			_, err = tracing.NewRemoteInstance(ctx, cl, instanceName,
				tracing.CPUBufferKiB(10240),
				tracing.InitialStop(),
				tracing.EnableEvents(strings.Split(traceCmdEvents, ",")...))
			if err != nil {
				s.Log("Failed to initialize TraceCmd: ", err)
				isTracingInstanceCreated = false
			}
			defer func(ctx context.Context) {
				if isTracingInstanceCreated {
					if err = tracing.CleanupRemoteInstance(ctx, cl, instanceName); err != nil {
						s.Log("Failed to clean the tracing remote instance: ", err)
					}
				}
			}(ctx)
			ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
			defer cancel()

			if err := tracing.StartRemoteInstanceTrace(ctx, cl, instanceName); err != nil {
				s.Log("Failed to start tracing: ", err)
				isTracingInstanceStarted = false
			}
			defer func(ctx context.Context) {
				if isTracingInstanceStarted {
					testScantype := "waifForScanFalse"
					if waitForScan {
						testScantype = "waifForScanTrue"
					}
					if s.HasError() {
						dest := fmt.Sprintf("%s/trace_%s_%s.dat", s.OutDir(), instanceName, testScantype)
						if err := tracing.SaveRemoteInstanceTraceData(ctx, cl, instanceName,
							func(src string) error {
								return s.DUT().GetFile(ctx, src, dest)
							}); err != nil {
							s.Log("Failed to copy the data file from DUT: ", err)
						} else {
							s.Logf("Save trace data into %q", dest)
						}
					}
				}
			}(ctx)
			_, cancel = ctxutil.Shorten(ctx, 3*time.Second)
			defer cancel()
		}

		apOpts1 := []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(1), hostapd.SpectrumManagement()}
		apOpts2 := []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(48), hostapd.SpectrumManagement()}
		params := s.Param().(bssTMReqTestCase)
		requestParams := params.requestParams
		if len(params.ap1Opts) > 0 {
			apOpts1 = params.ap1Opts
		}
		if len(params.ap2Opts) > 0 {
			apOpts2 = params.ap2Opts
		}
		if requestParams.DisassocImminent {
			apOpts1 = append(apOpts1, hostapd.MBO())
			apOpts2 = append(apOpts2, hostapd.MBO())
		}

		if params.pmfRequiredAP1 {
			apOpts1 = append(apOpts1, hostapd.PMF(hostapd.PMFRequired))
		}
		if params.pmfRequiredAP2 {
			apOpts2 = append(apOpts2, hostapd.PMF(hostapd.PMFRequired))
		}

		// Configure AP1, connect the DUT to it, then configure AP2.
		ap1Config := hostapd.ApConfig{ApOpts: apOpts1, SecConfFac: params.secConfFac1}
		ap2Config := hostapd.ApConfig{ApOpts: apOpts2, SecConfFac: params.secConfFac2}
		ctx, rt, finish, err := wifiutil.SimpleRoamInitialSetup(ctx, tf, []wificell.DutIdx{wificell.DefaultDUT}, ap1Config, ap2Config, false)
		if err != nil {
			s.Fatal("Failed initial setup of the test: ", err)
		}
		defer func() {
			if err := finish(); err != nil {
				s.Error("Error while tearing down test setup: ", err)
			}
		}()
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		fromBSSID := rt.AP1BSSID()
		roamBSSID := rt.AP2BSSID()
		testSSID := rt.AP1SSID()
		s.Log("AP 1 BSSID: ", fromBSSID)
		s.Log("AP 2 BSSID: ", roamBSSID)

		req := requestParams
		req.NeighborList = []hostapd.BSSTMNeighbor{{BSSID: roamBSSID, Channel: rt.AP2().Config().Channel}}

		if err := rt.SetupDUTForRoaming(ctx, wificell.DefaultDUT, fromBSSID, roamBSSID, testSSID, waitForScan); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		if err := rt.SendBSSTMReqAndWaitConnected(ctx, wificell.DefaultDUT, fromBSSID, roamBSSID, rt.AP1(), rt.AP2(), req, rt.ServicePathOfDUT(wificell.DefaultDUT), false); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		t := time.Now()

		err = tf.ClearBSSIDIgnoreDUT(ctx, wificell.DefaultDUT)
		if err != nil {
			s.Fatal("Failed to clear wpa BSSID_IGNORE: ", err)
		}

		if requestParams.DisassocImminent {
			// Test that the reassoc delay works as expected, and we
			// fail to reassoc to the original AP, and then sleep
			// until we are sure the delay has expired.
			// We expect the connection to fail, so send a request
			// without any additional parameters to test that the
			// connection fails. Otherwise, the reassoc delay will
			// disable the current AP as well and trigger a deauth.
			if err := rt.SendBSSTMReqAndWaitConnected(ctx, wificell.DefaultDUT, roamBSSID, fromBSSID, rt.AP2(), rt.AP1(), hostapd.BSSTMReqParams{NeighborList: []hostapd.BSSTMNeighbor{{BSSID: fromBSSID, Channel: rt.AP1().Config().Channel}}}, rt.ServicePathOfDUT(wificell.DefaultDUT), true); err != nil {
				s.Fatal("DUT: failed to roam and wait for connection: ", err)
			}
			if sleepDur := requestParams.ReassocDelay + bssTMReassocBuffer - time.Since(t); sleepDur > 0 {
				s.Log("Sleeping for ", sleepDur)
				// GoBigSleepLint this sleep is the part of the test design.
				if err := testing.Sleep(ctx, sleepDur); err != nil {
					s.Fatal("Failed to sleep: ", err)
				}
			}
		}

		if requestParams.BSSTerm > 0 || (params.secConfFac1 != nil && params.secConfFac2 != nil) {
			// Do not attempt to roam back if the AP was terminated or if security
			// configurations differ (e.g. WPA3-SAE to WPA2-PSK downgrade).
			return
		}

		req.NeighborList = []hostapd.BSSTMNeighbor{{BSSID: fromBSSID, Channel: rt.AP1().Config().Channel}}

		// Before we transition back to the original BSSID, add the current
		// BSSID into the ignore list. We add it twice purposely to ensure the
		// ignore duration is sufficient.
		err = tf.AddToBSSIDIgnoreDUT(ctx, wificell.DefaultDUT, roamBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}
		err = tf.AddToBSSIDIgnoreDUT(ctx, wificell.DefaultDUT, roamBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}

		if err := rt.SendBSSTMReqAndWaitConnected(ctx, wificell.DefaultDUT, roamBSSID, fromBSSID, rt.AP2(), rt.AP1(), req, rt.ServicePathOfDUT(wificell.DefaultDUT), false); err != nil {
			s.Fatal("DUT: failed to roam and wait for connection: ", err)
		}
		err = tf.ClearBSSIDIgnoreDUT(ctx, wificell.DefaultDUT)
		if err != nil {
			s.Fatal("Failed to clear wpa BSSID_IGNORE: ", err)
		}
	}

	// Before sending the BSS TM request, run a scan and make sure the DUT
	// has seen the second AP. In that case, the DUT will typically re-use
	// the result of the scan when receiving the request instead of probing
	// the second AP.
	if !s.Run(ctx, "waitForScan is true", func(ctx context.Context, s *testing.State) {
		runTest(ctx, s, true)
	}) {
		return
	}

	// After setting up both APs, immediately send the BSS TM Request before
	// the DUT has scanned and noticed the second AP (at least in the
	// majority of test runs). Instead of relying on the result of a previous
	// scan, the DUT will probe for the second AP when receiving the
	// transition request.
	if !s.Run(ctx, "waitForScan is false", func(ctx context.Context, s *testing.State) {
		runTest(ctx, s, false)
	}) {
		return
	}
}
