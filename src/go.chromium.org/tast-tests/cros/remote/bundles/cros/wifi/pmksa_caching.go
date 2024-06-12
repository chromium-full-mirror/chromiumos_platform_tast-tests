// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type pmksaCachingTestCase struct {
	apOpts     []hostapd.Option
	secConfFac security.ConfigFactory
	authAlgo   wpa.AuthAlgo // Authentication algorithm used if PMKSA caching doesn't present.
	checkEap   bool         // check EAP by monitoring wpa_supplicant signal in addition to checking pcap.
}

// EAP certs/keys for EAP tests.
var (
	cert = certificate.TestCert1()
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PMKSACaching,
		Desc: "Verifies that 802.1x authentication (EAP exchange) or non-Open authentication is bypassed if FT is enabled or PMKSA is done using PMK caching when it is available",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Val: pmksaCachingTestCase{
					secConfFac: wpaeap.NewConfigFactory(
						cert.CACred.Cert, cert.ServerCred,
						wpaeap.ClientCACert(cert.CACred.Cert),
						wpaeap.ClientCred(cert.ClientCred),
						wpaeap.Mode(wpa.ModePureWPA2),
					),
					authAlgo: wpa.AuthAlgoOpen,
					checkEap: true,
				},
				Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				Name:              "8021xwpa3",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				Val: pmksaCachingTestCase{
					secConfFac: wpaeap.NewConfigFactory(
						cert.CACred.Cert, cert.ServerCred,
						wpaeap.ClientCACert(cert.CACred.Cert),
						wpaeap.ClientCred(cert.ClientCred),
						wpaeap.Mode(wpa.ModePureWPA3),
					),
					authAlgo: wpa.AuthAlgoOpen,
					checkEap: true,
				},
				Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				Name:              "sae",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				Val: pmksaCachingTestCase{
					apOpts: []hostapd.Option{
						hostapd.PMF(hostapd.PMFRequired),
					},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA3),
						wpa.Ciphers2(wpa.CipherCCMP),
					),
					authAlgo: wpa.AuthAlgoSAE,
				},
				Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			{
				Name:              "ftsae",
				ExtraSoftwareDeps: []string{"wpa3_sae"},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiFT()),
				Val: pmksaCachingTestCase{
					apOpts: []hostapd.Option{
						hostapd.PMF(hostapd.PMFRequired),
					},
					secConfFac: wpa.NewConfigFactory(
						"chromeos", wpa.Mode(wpa.ModePureWPA3),
						wpa.Ciphers2(wpa.CipherCCMP), wpa.FTMode(wpa.FTModePure),
					),
					authAlgo: wpa.AuthAlgoFT,
				},
				Fixture:         wificell.FixtureID(wificell.TFFeaturesBridgeAndVeth),
				VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
			// wpa_supplicant disables PMKSA caching with FT-EAP by default due
			// to known interoperability issues with APs
		},
	})
}

func PMKSACaching(ctx context.Context, s *testing.State) {
	/*
		 This test verifies that 802.1X authentication (EAP exchange) or non-Open
		 authentication is bypassed
		 a. if FT is enabled
		 b. or PMKSA is done using PMK caching when it is available
		 by using the following steps:
		 1. Disable background and foreground scan so that the DUT doesn't
		    accidentally roam.
		 2. Set up an AP "AP0" using with random MAC/SSID and EAP or non-Open
		    authentication.
		 3. Connect the DUT to AP0.
		 4. Conduct a ping test to ensure we are connected to AP0.
		 5. Set up another AP "AP1" using random MAC, the same SSID, the same
		    EAP or non-Open authentication and WiFi mode as AP0.
		If FT is disabled:
		 6.1. While the DUT connected to AP0, request DUT to discover then roam
		      to AP1 using BSS TM Request.
		 6.2. Assert that the Shill property WiFiBSSID is equal to the BSSID from
		      AP1.
		 6.3. Assert that EAP or non-Open authentication is NOT skipped during the
		      roaming to AP1 if the test case is 802.1X or non-Open, respectively.
		 7.1. While the DUT connected to AP1, request DUT to roam to AP0 using
		      BSS TM Request.
		 7.2. Assert that the Shill property WiFiBSSID is equal to the BSSID from
		      AP0, which indicates a roaming.
		 7.3. Assert that EAP or non-Open authentication is skipped during the
		      roaming to AP0 if the test case is 802.1X or non-Open, respectively.
		 8.1. Deconfigure AP0.
		 8.2. Assert that the Shill property WiFiBSSID is equal to the BSSID from
		      AP1, which indicates a connection.
		 8.3. Assert that EAP or non-Open authentication is skipped during the
		      association to AP1.
		 9. Verify the DUT is connected to AP1 within timeout.
		 10. Clean up state and revert the steps from (1).
		If FT is enabled, the DUT uses FT instead of PMKSA caching during roaming,
		so PMKSA caching can only be verfied when AP1 is torn down and the DUT
		reconnects to AP0:
		 6.1. Deconfigure AP1.
		 6.2. Assert that the Shill property WiFiBSSID is equal to the BSSID from
		      AP0, which indicates a connection.
		 6.3. Assert that non-Open authentication is skipped during the association
		      to AP0.
		 7. Verify the DUT is connected to AP0 within timeout.
		 8. Clean up state and revert the steps from (1).
	*/
	const (
		ap0Channel  = 1
		ap1Channel  = 44
		roamTimeout = 30 * time.Second
	)

	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(pmksaCachingTestCase)
	pcapRouter := tf.PcapRouter()

	// checkAuthSkipped starts monitoring EAP or authentication before
	// invocation of action callback and check EAP or authentication outcome
	// after the completion of action.
	// If checkEap is true, then expectSkipped is used to check against EAP
	// flow, otherwise it's unused. Since EAP flow follows a successful Open
	// auth, checkEap=True implies there is an Open System auth flow in pcap.
	// Returns if EAP or non-Open authentication is skipped.
	checkAuthSkipped := func(ctx context.Context, s *testing.State, checkEap, expectSkipped bool, channel int, freqOps []iw.SetFreqOption, action func(context.Context) error) (*pcap.Capturer, error) {
		var skippedRecver func() (bool, error)
		// Do the prep work before invoking action.
		capturer, err := pcapRouter.StartCapture(ctx, "monitor"+tf.UniqueAPName(), channel, 0 /*opClass*/, freqOps)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start capturer")
		}
		defer func(ctx context.Context) {
			// Check the auth algo used in the authentication during
			// invocation of action.
			if err := pcapRouter.StopCapture(ctx, capturer); err != nil {
				s.Error("Failed to stop capturer: ", err)
			}
		}(ctx)
		ctx, cancel := pcapRouter.ReserveForStopCapture(ctx, capturer)
		defer cancel()

		// GoBigSleepLint. Wait a little while for the capturer to actually start capturing packets.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			return nil, errors.Wrap(err, "interrupted while sleeping for capturer startup")
		}
		if checkEap {
			skippedRecver, err = tf.WifiClient().EAPAuthSkipped(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create a EAP authentication watcher")
			}
		}

		if err := action(ctx); err != nil {
			return nil, err
		}

		if checkEap {
			skipped, err := skippedRecver()
			if err != nil {
				return nil, errors.Wrap(err, "failed to wait for confirming skipping EAP authentication")
			}
			if skipped != expectSkipped {
				return nil, errors.Errorf("EAP skipped=%t, expected=%t", skipped, expectSkipped)
			}
			s.Logf("EAP skipped=%t as expected", skipped)
		}
		return capturer, nil
	}

	checkAuthAlgorithm := func(ctx context.Context, s *testing.State, capturer *pcap.Capturer, expectedAuthAlgo wpa.AuthAlgo) error {
		authAlgo, err := tf.CheckFullAuthFlow(ctx, capturer)
		if err != nil {
			return errors.Wrap(err, "failed to determine the authentication algorithm from the pcap")
		}
		// Check if the outcome matches expectation.
		if authAlgo != expectedAuthAlgo {
			return errors.Errorf("authentication algorithm is %d, expected: %d", int(authAlgo), int(expectedAuthAlgo))
		}
		s.Logf("Authentication algorithm is %d as expected", int(authAlgo))
		return nil
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

	// Generate BSSIDs for the two APs.
	mac0, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate BSSID: ", err)
	}
	mac1, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate BSSID: ", err)
	}
	ap0BSSID := mac0.String()
	ap1BSSID := mac1.String()

	ssid := hostapd.RandomSSID("TAST_TEST_")

	// Generate hostapd options for APs.
	buildHostapdOpts := func(channel int, bssid string, extraOpts []hostapd.Option) (allApOpts []hostapd.Option) {
		allApOpts = []hostapd.Option{
			hostapd.SSID(ssid), hostapd.BSSID(bssid), hostapd.Mode(hostapd.Mode80211nPure),
			hostapd.Channel(channel), hostapd.HTCaps(hostapd.HTCapHT20),
		}
		allApOpts = append(allApOpts, extraOpts...)
		return
	}

	reconnectProps := func(bssid string) []*wificell.ShillProperty {
		return []*wificell.ShillProperty{{
			Property:       shillconst.ServicePropertyState,
			ExpectedValues: []interface{}{shillconst.ServiceStateReady},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		}, {
			Property:       shillconst.ServicePropertyWiFiBSSID,
			ExpectedValues: []interface{}{bssid},
			Method:         wifi.ExpectShillPropertyRequest_CHECK_ONLY,
		}}
	}

	ap0Opts := buildHostapdOpts(ap0Channel, ap0BSSID, tc.apOpts)
	ap1Opts := buildHostapdOpts(ap1Channel, ap1BSSID, tc.apOpts)

	var br []string
	var dhcpIface string
	ftEnabled := tc.authAlgo == wpa.AuthAlgoFT
	if ftEnabled {
		const (
			key0 = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
			key1 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
			mdID = "a1b2"
		)
		var (
			id0 = hex.EncodeToString(mac0)
			id1 = hex.EncodeToString(mac1)
		)
		br, err = tf.GetBridgesOnRouterID(wificell.DefaultRouter)
		if err != nil {
			s.Fatal("Failed to get bridge names on router: ", err)
		}
		dhcpIface = br[0]
		ap0Opts = append(ap0Opts, hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id0), hostapd.R1KeyHolder(id0),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", mac1, id1, key0)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", mac1, mac1, key1)),
			hostapd.Bridge(br[0]))
		ap1Opts = append(ap1Opts, hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id1), hostapd.R1KeyHolder(id1),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", mac0, id0, key1)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", mac0, mac0, key0)),
			hostapd.Bridge(br[1]))
	}

	ap0, err := tf.ConfigureAPOnRouterIDWithConfs(ctx, wificell.DefaultRouter, []hostapd.ApConfig{{ApOpts: ap0Opts, SecConfFac: tc.secConfFac}}, dhcpIface, true, false, false)
	if err != nil {
		s.Fatal("Failed to configure AP0: ", err)
	}
	defer func(ctx context.Context) {
		if ap0 == nil {
			// AP0 is already closed.
			return
		}
		if err := tf.DeconfigAP(ctx, ap0); err != nil {
			s.Error("Failed to deconfig AP0: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap0)
	defer cancel()

	s.Log("AP0 setup done; connecting")
	// Connect the DUT to AP0 using EAP or non-Open authentication.
	connResp, err := tf.ConnectWifiAP(ctx, ap0)
	if err != nil {
		s.Fatal("Failed to connect to AP0: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect AP0: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	if err := tf.PingFromDUT(ctx, ap0.ServerIP().String()); err != nil {
		s.Fatal("Failed to ping from the DUT: ", err)
	}

	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get interface from DUT: ", err)
	}

	ipr := ip.NewRemoteRunner(s.DUT().Conn())
	clientMAC, err := ipr.MAC(ctx, iface)
	if err != nil {
		s.Fatal("Failed to get MAC of WiFi interface: ", err)
	}

	ap1, err := tf.ConfigureAPOnRouterIDWithConfs(ctx, wificell.DefaultRouter, []hostapd.ApConfig{{ApOpts: ap1Opts, SecConfFac: tc.secConfFac}}, "", !ftEnabled, false, false)
	if err != nil {
		s.Fatal("Failed to configure AP1: ", err)
	}
	ap1FreqOps, err := ap1.Config().PcapFreqOptions()
	if err != nil {
		s.Fatal("Failed to get Freq Opts: ", err)
	}
	defer func(ctx context.Context) {
		if ap1 == nil {
			// AP1 is already closed.
			return
		}
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig AP1: ", err)
		}
	}(ctx)
	// Reserve time for deconfig ap1.
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()

	// Expect EAP or authentication is not skipped during DUT's association to
	// newly configured AP1.
	capturer, err := checkAuthSkipped(ctx, s, tc.checkEap, ftEnabled, ap1Channel, ap1FreqOps, func(actionCtx context.Context) error {
		// Expect the DUT to roam to AP1 within timeout.
		roamCtx, cancel := context.WithTimeout(ctx, roamTimeout)
		defer cancel()
		waitForRoam, err := tf.WifiClient().GenerateRoamPropertyWatcher(roamCtx, ap1BSSID, connResp.ServicePath)
		if err != nil {
			return errors.Wrap(err, "failed to create a property watcher on DUT")
		}

		req := hostapd.BSSTMReqParams{
			DisassocImminent: true,
			Neighbors:        []string{ap1BSSID},
		}
		s.Logf("Sending BSS Transition Management Request from AP0 %s to DUT %s", mac0, clientMAC)
		if err := ap0.SendBSSTMRequest(actionCtx, clientMAC.String(), req); err != nil {
			return errors.Wrap(err, "failed to send BSS TM Request")
		}

		s.Log("Waiting for roaming to AP1")
		if _, err := waitForRoam(); err != nil {
			return errors.Wrap(err, "failed to wait for roaming to AP1")
		}
		return nil
	})
	if err != nil {
		s.Fatal("Failed to roam to AP1 as expected: ", err)
	}
	if err := checkAuthAlgorithm(ctx, s, capturer, tc.authAlgo); err != nil {
		s.Fatal("The authentication algorithm is unexpected: ", err)
	}

	ap0FreqOps, err := ap0.Config().PcapFreqOptions()
	if err != nil {
		s.Fatal("Failed to get Freq Opts: ", err)
	}

	if !ftEnabled {
		// Expect EAP or authentication is skipped during DUT roaming back to
		// previously connected AP0.
		capturer, err := checkAuthSkipped(ctx, s, tc.checkEap, true, ap0Channel, ap0FreqOps, func(actionCtx context.Context) error {
			// Expect the DUT to roam to AP0 within timeout.
			roamCtx, cancel := context.WithTimeout(ctx, roamTimeout)
			defer cancel()
			waitForRoam, err := tf.WifiClient().GenerateRoamPropertyWatcher(roamCtx, ap0BSSID, connResp.ServicePath)
			if err != nil {
				return errors.Wrap(err, "failed to create a property watcher on DUT")
			}

			req := hostapd.BSSTMReqParams{
				DisassocImminent: true,
				Neighbors:        []string{ap0BSSID},
			}
			s.Logf("Sending BSS Transition Management Request from AP1 %s to DUT %s", mac1, clientMAC)
			if err := ap1.SendBSSTMRequest(actionCtx, clientMAC.String(), req); err != nil {
				return errors.Wrap(err, "failed to send BSS TM Request")
			}

			s.Log("Waiting for falling back to AP0")
			if _, err := waitForRoam(); err != nil {
				return errors.Wrap(err, "failed to wait for falling back to AP0")
			}
			return nil
		})
		if err != nil {
			s.Fatal("Failed to roam to AP0 as expected: ", err)
		}
		if err := checkAuthAlgorithm(ctx, s, capturer, wpa.AuthAlgoOpen); err != nil {
			s.Fatal("The authentication algorithm is unexpected: ", err)
		}

		// Expect EAP or authentication is skipped during DUT reconnecting to
		// previously connected AP1 after deconfiguring AP0.
		capturer, err = checkAuthSkipped(ctx, s, tc.checkEap, true, ap1Channel, ap1FreqOps, func(actionCtx context.Context) error {
			// Expect the DUT to reconnect to AP1 within timeout.
			reconnectCtx, cancel := context.WithTimeout(ctx, roamTimeout)
			defer cancel()
			waitForReconnect, err := tf.WifiClient().ExpectShillProperty(reconnectCtx, connResp.ServicePath, reconnectProps(ap1BSSID), nil)
			if err != nil {
				return errors.Wrap(err, "failed to create a property watcher on DUT")
			}

			s.Log("Tearing down AP0")
			if err := tf.DeconfigAP(actionCtx, ap0); err != nil {
				return errors.Wrap(err, "failed to deconfig AP0")
			}
			ap0 = nil

			s.Log("Waiting for falling back to AP1")
			if _, err := waitForReconnect(); err != nil {
				return errors.Wrap(err, "failed to wait for falling back to AP1")
			}
			return nil
		})
		if err != nil {
			s.Fatal("Failed to fall back to AP1 as expected: ", err)
		}
		if err := checkAuthAlgorithm(ctx, s, capturer, wpa.AuthAlgoOpen); err != nil {
			s.Fatal("The authentication algorithm is unexpected: ", err)
		}
		if err = tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap1); err != nil {
			s.Fatal("Failed to wait for the connection to recover: ", err)
		}
	} else {
		// Expect authentication is skipped during DUT reconnecting to
		// previously connected AP0 after deconfiguring AP1.
		capturer, err := checkAuthSkipped(ctx, s, tc.checkEap, true, ap0Channel, ap0FreqOps, func(actionCtx context.Context) error {
			reconnectCtx, cancel := context.WithTimeout(ctx, roamTimeout)
			defer cancel()
			waitForReconnect, err := tf.WifiClient().ExpectShillProperty(reconnectCtx, connResp.ServicePath, reconnectProps(ap0BSSID), nil)
			if err != nil {
				return errors.Wrap(err, "failed to create a property watcher on DUT")
			}

			s.Log("Tearing down AP1")
			if err := tf.DeconfigAP(actionCtx, ap1); err != nil {
				return errors.Wrap(err, "failed to deconfig AP1")
			}
			ap1 = nil

			s.Log("Waiting for falling back to AP0")
			if _, err := waitForReconnect(); err != nil {
				return errors.Wrap(err, "failed to wait for falling back to AP0")
			}
			return nil
		})
		if err != nil {
			s.Fatal("Failed to fall back  to AP0 as expected: ", err)
		}
		if err := checkAuthAlgorithm(ctx, s, capturer, wpa.AuthAlgoOpen); err != nil {
			s.Fatal("The authentication algorithm is unexpected: ", err)
		}
		if err = tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap0); err != nil {
			s.Fatal("Failed to wait for the connection to recover: ", err)
		}
	}
}
