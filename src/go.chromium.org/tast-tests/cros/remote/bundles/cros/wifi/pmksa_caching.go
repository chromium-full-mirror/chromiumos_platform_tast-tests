// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/network/iw"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
)

type pmksaCachingTestCase struct {
	apOpts     []hostapd.Option
	secOpts    []wpaeap.Option
	secConfFac security.ConfigFactory
	authAlgo   wpa.AuthAlgo // Authentication algorithm used if PMKSA caching doesn't present.
	checkEap   bool         // check EAP by monitoring wpa_supplicant signal in addition to checking pcap.
}

// EAP certs/keys for EAP tests.
var (
	cert          = certificate.TestCert1()
	requestParams = hostapd.BSSTMReqParams{
		DisassocImminent: true,
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PMKSACaching,
		Desc: "Verifies that 802.1x authentication (EAP exchange) or non-Open authentication is bypassed and PMKSA is done using PMK caching when it is available",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"junyuu@chromium.org",             // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// TODO: b/315386450 - Stabilize wifi.PMKSACaching test
		Attr:            []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
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
			},
		},
	})
}

func PMKSACaching(ctx context.Context, s *testing.State) {
	/*
	 This test verifies that 802.1X authentication (EAP exchange) or non-Open
	 authentication is bypassed and PMKSA is done using PMK caching when it
	 is available by using the following steps:
	 1. Disable background and foreground scan so that the DUT doesn't
	    accidentally roam.
	 2. Set up an AP "AP0" using with random MAC/SSID and EAP or non-Open
	    authentication.
	 3. Connect the DUT to AP0.
	 4. Conduct a ping test to ensure we are connected to AP0.
	 5. Set up another AP "AP1" using random MAC, the same SSID, the same
	    EAP or non-Open authentication and WiFi mode as AP0.
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
	      AP1, which indicates a roaming.
	 8.3. Assert that EAP or non-Open authentication is skipped during the
	      association to AP1.
	 9. Verify the DUT is connected to AP1 within timeout.
	 10. Clean up state and revert the steps from (1).
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
	checkAuthSkipped := func(ctx context.Context, s *testing.State, expectedAuthAlgo wpa.AuthAlgo, checkEap, expectSkipped bool, channel int, freqOps []iw.SetFreqOption, action func(context.Context)) {
		var skippedRecver func() (bool, error)
		// Do the prep work before invoking action.
		capturer, err := pcapRouter.StartCapture(ctx, "monitor"+tf.UniqueAPName(), channel, 0 /*opClass*/, freqOps)
		if err != nil {
			s.Fatal("Failed to start capturer: ", err)
		}
		if checkEap {
			expectedAuthAlgo = wpa.AuthAlgoOpen
			skippedRecver, err = tf.WifiClient().EAPAuthSkipped(ctx)
			if err != nil {
				s.Fatal("Failed to create a EAP authentication watcher: ", err)
			}
		}

		action(ctx)

		// Check the auth algo used in the authentication during
		// invocation of action.
		ctx, cancel := pcapRouter.ReserveForStopCapture(ctx, capturer)
		defer cancel()
		if err := pcapRouter.StopCapture(ctx, capturer); err != nil {
			s.Error("Failed to stop capturer: ", err)

		}
		authAlgo, err := tf.CheckFullAuthFlow(ctx, capturer)
		if err != nil {
			s.Fatal("Failed to check authentication flow in pcap")
		}

		// Check if the outcome matches expectation.
		if authAlgo != expectedAuthAlgo {
			s.Errorf("Authentication algorithm is %d, expected: %d", int(authAlgo), int(expectedAuthAlgo))
		} else {
			s.Logf("Authentication algorithm is %d as expected", int(authAlgo))
		}

		if checkEap {
			skipped, err := skippedRecver()
			if err != nil {
				s.Fatal("Failed to wait for confirming skipping EAP authentication: ", err)
			}

			if skipped != expectSkipped {
				s.Errorf("EAP skipped=%t, expected=%t", skipped, expectSkipped)
			} else {
				s.Logf("EAP skipped=%t as expected", skipped)
			}
		}
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

	roamProps := func(bssid string) []*wificell.ShillProperty {
		return []*wificell.ShillProperty{{
			Property:       shillconst.ServicePropertyWiFiBSSID,
			ExpectedValues: []interface{}{bssid},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		}}
	}

	ap0Opts := buildHostapdOpts(ap0Channel, ap0BSSID, tc.apOpts)
	ap1Opts := buildHostapdOpts(ap1Channel, ap1BSSID, tc.apOpts)

	ap0, err := tf.ConfigureAP(ctx, ap0Opts, tc.secConfFac)
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

	var ap1 *wificell.APIface
	// ap1Ctx should only be used by configure/deconfigure AP1.
	ap1Ctx := ctx

	defer func(ctx context.Context) {
		if ap1 == nil {
			// AP0 is already closed.
			return
		}
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig AP1: ", err)
		}
	}(ap1Ctx)
	// Reserve time for deconfig ap1. Note that ap1 should be created
	// after the waitForRoam property watcher, we borrow ap0 to
	// reserve time for DeconfigAP.
	_, cancel = tf.ReserveForDeconfigAP(ap1Ctx, ap0)
	defer cancel()

	s.Log("AP0 setup done; connecting")
	// Connect the DUT to AP0 using EAP or non-Open authentication.
	connResp, err := tf.ConnectWifiAP(ctx, ap0)
	if err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	if err := tf.PingFromDUT(ctx, ap0.ServerIP().String()); err != nil {
		s.Fatal("Failed to ping from the DUT: ", err)
	}

	// Expect the DUT to roam to AP1 within timeout.
	roamCtx, cancel := context.WithTimeout(ctx, roamTimeout)
	defer cancel()
	waitForRoam, err := tf.WifiClient().ExpectShillProperty(roamCtx, connResp.ServicePath, roamProps(ap1BSSID), nil)
	if err != nil {
		s.Fatal("Failed to create a property watcher on DUT: ", err)
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

	// AP1 is yet configured. To get AP1's PcapFreqOptions early, generate a Config
	securityConfig, err := tc.secConfFac.Gen()
	if err != nil {
		s.Fatal("Failed to generate SecurityConfig: ", err)
	}
	ap1Config, err := hostapd.NewConfig(append([]hostapd.Option{hostapd.SecurityConfig(securityConfig)}, ap1Opts...)...)
	if err != nil {
		s.Fatal("Failed to get hostapd Config: ", err)
	}
	ap1FreqOps, err := ap1Config.PcapFreqOptions()
	if err != nil {
		s.Fatal("Failed to get Freq Opts: ", err)
	}

	// Expect EAP or authentication is not skipped during DUT's association to
	// newly configured AP1.
	checkAuthSkipped(roamCtx, s, tc.authAlgo, tc.checkEap, false, ap1Channel, ap1FreqOps, func(actionCtx context.Context) {
		// Configure AP1 after ExpectShillProperty() because a roaming
		// may happen automatically right after AP1 is up.
		ap1, err = tf.ConfigureAP(actionCtx, ap1Opts, tc.secConfFac)
		if err != nil {
			s.Fatal("Failed to configure AP1: ", err)
		}
		if err := tf.WifiClient().DiscoverBSSID(actionCtx, ap1BSSID, iface, []byte(ssid)); err != nil {
			s.Fatal("Failed to discover AP1's BSSID: ", err)
		}

		req := requestParams
		req.Neighbors = []string{ap1BSSID}

		s.Logf("Sending BSS Transition Management Request from AP0 %s to DUT %s", mac0, clientMAC)
		if err := ap0.SendBSSTMRequest(actionCtx, clientMAC.String(), req); err != nil {
			s.Fatal("Failed to send BSS TM Request: ", err)
		}

		s.Log("Waiting for roaming to AP1")
		if _, err := waitForRoam(); err != nil {
			s.Fatal("Failed to wait for roaming to AP1: ", err)
		}
	})

	// Expect the DUT to roam to AP0 within timeout.
	roamCtx, cancel = context.WithTimeout(ctx, roamTimeout)
	defer cancel()
	waitForRoam, err = tf.WifiClient().ExpectShillProperty(roamCtx, connResp.ServicePath, roamProps(ap0BSSID), nil)
	if err != nil {
		s.Fatal("Failed to create a property watcher on DUT: ", err)
	}

	ap0FreqOps, err := ap0.Config().PcapFreqOptions()
	if err != nil {
		s.Fatal("Failed to get Freq Opts: ", err)
	}

	// Expect EAP or authentication is skipped during DUT roaming back to
	// previously connected AP0.
	checkAuthSkipped(roamCtx, s, wpa.AuthAlgoOpen, tc.checkEap, true, ap0Channel, ap0FreqOps, func(actionCtx context.Context) {
		req := requestParams
		req.Neighbors = []string{ap0BSSID}

		s.Logf("Sending BSS Transition Management Request from AP1 %s to DUT %s", mac1, clientMAC)
		if err := ap1.SendBSSTMRequest(actionCtx, clientMAC.String(), req); err != nil {
			s.Fatal("Failed to send BSS TM Request: ", err)
		}

		s.Log("Waiting for falling back to AP0")
		if _, err := waitForRoam(); err != nil {
			s.Fatal("Failed to wait for falling back to AP0: ", err)
		}
	})

	// Expect the DUT to roam to AP1 within timeout.
	roamCtx, cancel = context.WithTimeout(ctx, roamTimeout)
	defer cancel()
	waitForRoam, err = tf.WifiClient().ExpectShillProperty(roamCtx, connResp.ServicePath, roamProps(ap1BSSID), nil)
	if err != nil {
		s.Fatal("Failed to create a property watcher on DUT: ", err)
	}

	// Expect EAP or authentication is skipped during DUT roaming back to
	// previously connected AP1 after deconfiguring AP0.
	checkAuthSkipped(roamCtx, s, wpa.AuthAlgoOpen, tc.checkEap, true, ap1Channel, ap1FreqOps, func(actionCtx context.Context) {
		if err := tf.DeconfigAP(actionCtx, ap0); err != nil {
			s.Fatal("Failed to deconfig AP: ", err)
		}
		ap0 = nil

		s.Log("Waiting for falling back to AP1")
		if _, err := waitForRoam(); err != nil {
			s.Fatal("Failed to wait for falling back to AP1: ", err)
		}
	})

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return tf.VerifyConnection(ctx, ap1)
	}, &testing.PollOptions{
		Timeout:  time.Second * 20,
		Interval: time.Second,
	}); err != nil {
		s.Error("Failed to wait for the connection to recover: ", err)
	}

	s.Log("Tearing down")
}
