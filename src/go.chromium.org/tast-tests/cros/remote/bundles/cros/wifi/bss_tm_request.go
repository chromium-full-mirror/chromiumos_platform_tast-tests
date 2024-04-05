// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	bssTMRoamTimeout = 30 * time.Second
	// Give a 20 second buffer to make sure we can attempt to roam back to
	// the original AP before the retry delay is over.
	bssTMReassocDelay  = bssTMRoamTimeout + 20*time.Second
	bssTMReassocBuffer = 5 * time.Second
)

type bssTMReqTestCase struct {
	// requestParams defines the parameters for the BSS Transition Management Request.
	requestParams hostapd.BSSTMReqParams
	// secConfFac0 is the security configuration factory for the first AP.
	secConfFac0 security.ConfigFactory
	// secConfFac1 is the security configuration factory for the second AP.
	secConfFac1 security.ConfigFactory
	// pmfRequiredAP0 indicates whether AP 0 should enable the Hostapd config |PMFRequired|.
	pmfRequiredAP0 bool
	// pmfRequiredAP1 indicates whether AP 1 should enable the Hostapd config |PMFRequired|.
	pmfRequiredAP1 bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BSSTMRequest,
		Desc: "Tests the DUTs response to a BSS Transition Management Request",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
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
						DisassocTimer:    bssTMRoamTimeout,
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
					secConfFac0:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					secConfFac1:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					pmfRequiredAP1: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				// Verifies that DUT can roam from a BSS with SAE key management to a BSS with PSK key management and back.
				Name: "sae_to_psk",
				Val: bssTMReqTestCase{
					secConfFac0:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
					secConfFac1:    wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					pmfRequiredAP0: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
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
	2- Set up an AP "AP0" using security configs from |secConfFac0| if present.
	3- Connect to AP0.
	4- Set up another AP "AP1" using security configs from |secConfFac1| if present.
	5- Add the BSSID at AP0 into the DUT's ignorelist.
	6- Send a BSSTM request from AP0.
	7- Assert that the Shill property RoamState transitions from configuration
		-> ready -> idle, which indicates a roam has occurred.
	8- Assert that the BSSID property in Shill is equal to the BSSID from AP1.
	9- Conduct a ping test to ensure we are connected to AP1.
	10- If "disassoc_imminent" is enabled, send another BSSTM Request from AP1,
		but this time assert that the roam fails due to the reassociation
		delay.
	11- Send a BSSTM Request from AP1 without dissasoc imminent.
	12- Assert that the Shill property RoamState transitions from configuration
		-> ready -> idle, which indicates a roam has occurred.
	13- Assert that the BSSID property in Shill is equal to the BSSID from AP0.
	14- Conduct a ping test to ensure we are connected to AP0.
	15- Clean up state and revert the steps from (1).
*/

func BSSTMRequest(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	allowRoamResp, err := tf.WifiClient().GetScanAllowRoamProperty(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get the ScanAllowRoam property: ", err)
	}
	if allowRoamResp.Allow {
		if _, err := tf.WifiClient().SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: false}); err != nil {
			s.Error("Failed to set ScanAllowRoam property to false: ", err)
		}
		defer func(ctx context.Context) {
			if _, err := tf.WifiClient().SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: allowRoamResp.Allow}); err != nil {
				s.Errorf("Failed to set ScanAllowRoam property back to %v: %v", allowRoamResp.Allow, err)
			}
		}(ctx)
	}

	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	runTest := func(ctx context.Context, s *testing.State, waitForScan bool) {
		// Generate BSSIDs for the two APs.
		mac0, err := hostapd.RandomMAC()
		if err != nil {
			s.Fatal("Failed to generate BSSID: ", err)
		}
		mac1, err := hostapd.RandomMAC()
		if err != nil {
			s.Fatal("Failed to generate BSSID: ", err)
		}
		fromBSSID := mac0.String()
		roamBSSID := mac1.String()
		s.Log("AP 0 BSSID: ", fromBSSID)
		s.Log("AP 1 BSSID: ", roamBSSID)

		testSSID := hostapd.RandomSSID("BSS_TM_")
		apOpts0 := []hostapd.Option{hostapd.SSID(testSSID), hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(1), hostapd.BSSID(fromBSSID)}
		apOpts1 := []hostapd.Option{hostapd.SSID(testSSID), hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(48), hostapd.BSSID(roamBSSID)}
		params := s.Param().(bssTMReqTestCase)
		requestParams := params.requestParams
		if requestParams.DisassocImminent {
			apOpts0 = append(apOpts0, hostapd.MBO())
			apOpts1 = append(apOpts1, hostapd.MBO())
		}

		if params.pmfRequiredAP0 {
			apOpts0 = append(apOpts0, hostapd.PMF(hostapd.PMFRequired))
		}
		if params.pmfRequiredAP1 {
			apOpts1 = append(apOpts1, hostapd.PMF(hostapd.PMFRequired))
		}

		// Configure the first AP.
		s.Log("Configuring AP 0")
		ap0, err := tf.ConfigureAP(ctx, apOpts0, params.secConfFac0)
		if err != nil {
			s.Fatal("Failed to configure AP 0: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap0); err != nil {
				s.Error("Failed to deconfig AP 0: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap0)
		defer cancel()

		// Connect to the first AP.
		s.Log("Connecting to AP 0")
		cleanupCtx := ctx
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		connectResp, err := tf.ConnectWifiAP(ctx, ap0)
		if err != nil {
			s.Fatal("Failed to connect to AP 0: ", err)
		}
		servicePath := connectResp.ServicePath
		defer func(ctx context.Context) {
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
		}(cleanupCtx)
		s.Log("Verifying connection to AP 0")
		if err := tf.VerifyConnection(ctx, ap0); err != nil {
			s.Fatal("Failed to verify connection: ", err)
		}

		// Set up a second AP with the same SSID.
		s.Log("Configuring AP 1")
		ap1, err := tf.ConfigureAP(ctx, apOpts1, params.secConfFac1)
		if err != nil {
			s.Fatal("Failed to configure AP 1: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap1); err != nil {
				s.Error("Failed to deconfig AP 1: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap1)
		defer cancel()

		// Get the name and MAC address of the DUT WiFi interface.
		clientIface, err := tf.ClientInterface(ctx)
		if err != nil {
			s.Fatal("Unable to get DUT interface name: ", err)
		}
		clientMAC, err := tf.ClientHardwareAddr(ctx)
		if err != nil {
			s.Fatal("Unable to get DUT MAC address: ", err)
		}

		// Flush all scanned BSS from wpa_supplicant so that test behavior is consistent.
		s.Log("Flushing BSS cache")
		if err := tf.WifiClient().FlushBSS(ctx, clientIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}

		// Wait for roamBSSID to be discovered if waitForScan is set.
		if waitForScan {
			s.Logf("Waiting for roamBSSID: %s", roamBSSID)
			if err := tf.WifiClient().DiscoverBSSID(ctx, roamBSSID, clientIface, []byte(testSSID)); err != nil {
				s.Fatal("Unable to discover roam BSSID: ", err)
			}
		}

		// Set up a watcher for the Shill WiFi BSSID property.
		waitCtx, cancel := context.WithTimeout(ctx, bssTMRoamTimeout)
		defer cancel()
		waitForProps, err := tf.WifiClient().GenerateRoamPropertyWatcher(waitCtx, roamBSSID, servicePath)

		sendReqAndWaitConnected := func(from, to string, fromAP, toAP *wificell.APIface, req hostapd.BSSTMReqParams, expectConnectFail bool) {
			// Send BSS Transition Management Request to client.
			s.Logf("Sending BSS Transition Management Request from AP %s to DUT %s", from, clientMAC)
			if err := fromAP.SendBSSTMRequest(ctx, clientMAC, req); err != nil {
				s.Fatal("Failed to send BSS TM Request: ", err)
			}

			// Wait for the DUT to roam to the second AP, then assert that there was
			// no disconnection during roaming.
			s.Log("Waiting for roaming")
			monitorResult, err := waitForProps()
			if err != nil {
				if expectConnectFail {
					s.Log("Connection failed as expected")
					return
				}
				s.Fatal("Failed to roam within timeout: ", err)
			}
			if expectConnectFail {
				s.Fatal("Expected roam to fail but it succeeded")
			}
			for _, ph := range monitorResult {
				if ph.Name == shillconst.ServicePropertyIsConnected {
					if !ph.Value.(bool) {
						s.Fatal("Failed to stay connected during the roaming process")
					}
				}
			}

			// Just for good measure make sure we're properly connected.
			s.Log("Verifying connection to AP ", to)
			if err := tf.VerifyConnection(ctx, toAP); err != nil {
				s.Fatal("DUT: failed to verify connection: ", err)
			}
		}

		req := requestParams
		req.Neighbors = []string{roamBSSID}
		err = tf.ClearBSSIDIgnoreDUT(ctx, wificell.DefaultDUT)
		if err != nil {
			s.Fatal("Failed to clear wpa BSSID_IGNORE: ", err)
		}
		// Before sending the BSSTM request, add the current BSSID into the
		// DUT's ignorelist to avoid any any potential race condition. Adding a
		// BSSID to the ignore list does not trigger the device to roam away
		// from the BSSID, but it should prevent it from roaming back.
		// NB: Each time we add the BSSID to the ignore list, it increases
		// the duration for which the BSSID is ignored. Each wpa_cli
		// invocation results in the BSSID being added to the ignore list
		// twice, so the two calls here translate to 4 insertions in
		// wpa_supplicant, which results in an ignorelist duration of 120
		// seconds, which should be plenty.

		// We add the BSSID into the ignorelist twice purposely to ensure the
		// ignore duration is sufficient.
		err = tf.AddToBSSIDIgnoreDUT(ctx, wificell.DefaultDUT, fromBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}
		err = tf.AddToBSSIDIgnoreDUT(ctx, wificell.DefaultDUT, fromBSSID)
		if err != nil {
			s.Fatal("Failed to add wpa BSSID_IGNORE: ", err)
		}
		sendReqAndWaitConnected(fromBSSID, roamBSSID, ap0, ap1, req, false)
		t := time.Now()

		waitCtx, cancel = context.WithTimeout(ctx, bssTMRoamTimeout)
		defer cancel()
		waitForProps, err = tf.WifiClient().GenerateRoamPropertyWatcher(waitCtx, fromBSSID, servicePath)
		if err != nil {
			s.Fatal("Failed to create Shill property watcher: ", err)
		}
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
			sendReqAndWaitConnected(roamBSSID, fromBSSID, ap1, ap0, hostapd.BSSTMReqParams{Neighbors: []string{fromBSSID}}, true)
			if sleepDur := requestParams.ReassocDelay + bssTMReassocBuffer - time.Now().Sub(t); sleepDur > 0 {
				s.Log("Sleeping for ", sleepDur)
				// GoBigSleepLint this sleep is the part of the test design.
				if err := testing.Sleep(ctx, sleepDur); err != nil {
					s.Fatal("Failed to sleep: ", err)
				}
			}

			waitCtx, cancel = context.WithTimeout(ctx, bssTMRoamTimeout)
			defer cancel()
			waitForProps, err = tf.WifiClient().GenerateRoamPropertyWatcher(waitCtx, fromBSSID, servicePath)
			if err != nil {
				s.Fatal("Failed to create Shill property watcher: ", err)
			}
		}
		req.Neighbors = []string{fromBSSID}

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
		sendReqAndWaitConnected(roamBSSID, fromBSSID, ap1, ap0, req, false)
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
