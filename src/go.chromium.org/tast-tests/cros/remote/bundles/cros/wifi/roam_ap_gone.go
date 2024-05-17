// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type roamTestcase struct {
	apOpts1          []hostapd.Option
	apOpts2          []hostapd.Option
	secConfFac       security.ConfigFactory
	enableBSSFlush   bool
	expectedRoamTime time.Duration
}

// EAP certs/keys for EAP tests.
var (
	roamCert = certificate.TestCert1()
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RoamAPGone,
		Desc: "Tests roaming to an AP that disappears while the client is awake",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"rmekonnen@google.com",
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				// Verifies that DUT can roam between two APs in full view of it.
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       nil,
					enableBSSFlush:   false,
					expectedRoamTime: 10 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it.
				Name: "wpa",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					enableBSSFlush:   false,
					expectedRoamTime: 10 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WEP APs in full view of it.
				Name: "wep",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wep.NewConfigFactory([]string{"abcde", "fedcba9876", "ab\xe4\xb8\x89", "\xe4\xb8\x89\xc2\xa2"}, wep.DefaultKey(0), wep.AuthAlgs(wep.AuthAlgoOpen)),
					enableBSSFlush:   false,
					expectedRoamTime: 10 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiWEP(), hwdep.WifiNotMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWEP},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA-EAP APs in full view of it.
				Name: "8021xwpa",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wpaeap.NewConfigFactory(roamCert.CACred.Cert, roamCert.ServerCred, wpaeap.ClientCACert(roamCert.CACred.Cert), wpaeap.ClientCred(roamCert.ClientCred)),
					enableBSSFlush:   false,
					expectedRoamTime: 10 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				Name: "flushbss",
				// Verifies that DUT can roam between two APs with minimal idle time after bss flush.
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       nil,
					enableBSSFlush:   true,
					expectedRoamTime: 10 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				Name: "marvell",
				// Verifies that DUT can roam between two APs in full view of it.
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       nil,
					enableBSSFlush:   false,
					expectedRoamTime: 12 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA APs in full view of it.
				Name: "wpa_marvell",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP)),
					enableBSSFlush:   false,
					expectedRoamTime: 12 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WEP APs in full view of it.
				Name: "wep_marvell",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nMixed), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wep.NewConfigFactory([]string{"abcde", "fedcba9876", "ab\xe4\xb8\x89", "\xe4\xb8\x89\xc2\xa2"}, wep.DefaultKey(0), wep.AuthAlgs(wep.AuthAlgoOpen)),
					enableBSSFlush:   false,
					expectedRoamTime: 12 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiWEP(), hwdep.WifiMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWEP},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				// Verifies that DUT can roam between two WPA-EAP APs in full view of it.
				Name: "8021xwpa_marvell",
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       wpaeap.NewConfigFactory(roamCert.CACred.Cert, roamCert.ServerCred, wpaeap.ClientCACert(roamCert.CACred.Cert), wpaeap.ClientCred(roamCert.ClientCred)),
					enableBSSFlush:   false,
					expectedRoamTime: 12 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				ExtraRequirements: []string{tdreq.WiFiSecSupportWPA2Enterprise},
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			}, {
				Name: "flushbss_marvell",
				// Verifies that DUT can roam between two APs with minimal idle time after bss flush.
				Val: roamTestcase{
					apOpts1:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)},
					apOpts2:          []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)},
					secConfFac:       nil,
					enableBSSFlush:   true,
					expectedRoamTime: 12 * time.Second,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				VariantCategory:   `{"name": "WifiBtChipset_Soc_Kernel"}`,
			},
		},
	})
}

func RoamAPGone(ctx context.Context, s *testing.State) {
	/*
		This test checks a DUT's ability to naturally roam after AP lost
		by using the following steps:
		1 - Configure AP1.
		2 - Associate DUT to AP1.
		3 - Configure AP2 with the same SSID as AP1.
		4 - Either flush all BSSes to force the DUT to rescan after AP1
		    disappears or trigger a new scan now, so that the DUT can
		    autoconnect after AP1 disappears.
		5 - Deconfigure AP1.
		6 - Verify the DUT roams to AP2.
		7 - Deconfigure the DUT.
		8 - Deconfigure AP2.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	// Configure the initial AP.
	param := s.Param().(roamTestcase)
	ap1, err := tf.ConfigureAP(ctx, param.apOpts1, param.secConfFac)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	ssid := ap1.Config().SSID
	defer func(ctx context.Context) {
		if ap1 == nil {
			// ap1 is already deconfigured.
			return
		}
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig ap1, err: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()
	s.Log("AP1 setup done")

	// Schedule defer for AP2 before connection. Otherwise, we
	// will teardown AP2 before disconnect, and the DUT might
	// get disconnected due to inactivity and causing flaky
	// Disconnect failure.
	var ap2 *wificell.APIface
	defer func(ctx context.Context) {
		if ap2 == nil {
			return
		}
		if err := tf.DeconfigAP(ctx, ap2); err != nil {
			s.Error("Failed to deconfig ap2, err: ", err)
		}
	}(ctx)
	// We don't have ap2 yet, borrow the reserve of ap1.
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()

	// Connect to the initial AP.
	var servicePath string
	if resp, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap1); err != nil {
		s.Fatal("Failed to connect to WiFi, err: ", err)
	} else {
		servicePath = resp.ServicePath
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	s.Log("Connected to AP1")

	if err := tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap1); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}

	// Generate the BSSID for second AP.
	mac, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	ap2BSSID := mac.String()

	props := []*wificell.ShillProperty{
		{
			Property:       shillconst.ServicePropertyWiFiBSSID,
			ExpectedValues: []interface{}{ap2BSSID},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		},
	}

	// Configure the second AP.
	var ops []hostapd.Option
	ops = append(ops, param.apOpts2...)
	// Override SSID and BSSID as we need the same SSID as the first AP
	// and the BSSID that we're waiting.
	ops = append(ops, hostapd.SSID(ssid), hostapd.BSSID(ap2BSSID))
	ap2, err = tf.ConfigureAP(ctx, ops, param.secConfFac)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	// defer deconfig already scheduled above.
	s.Log("AP2 setup done")

	clientIface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("Unable to get DUT interface name: ", err)
	}

	if param.enableBSSFlush {
		// Flush all BSSes from cache to ensure that we are forced to rescan
		// after disconnect.
		s.Log("Flushing BSS cache")
		if err := tf.DUTWifiClient(wificell.DefaultDUT).FlushBSS(ctx, clientIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}
	} else {
		// Discover AP2 before disconnecting from AP1.
		s.Logf("Waiting for AP2 discovery: %s", ap2BSSID)
		if err := tf.DUTWifiClient(wificell.DefaultDUT).DiscoverBSSID(ctx, ap2BSSID, clientIface, []byte(ssid)); err != nil {
			s.Fatal("Unable to discover AP2 BSSID: ", err)
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	waitForProps, err := tf.WifiClient().ExpectShillProperty(waitCtx, servicePath, props, nil)
	if err != nil {
		s.Fatal("DUT: failed to create a property watcher, err: ", err)
	}
	startTime := time.Now()

	// Deconfigure the initial AP.
	if err := tf.DeconfigAP(ctx, ap1); err != nil {
		s.Error("Failed to deconfig ap, err: ", err)
	}
	ap1 = nil
	s.Log("Deconfigured AP1")

	if _, err := waitForProps(); err != nil {
		s.Fatal("DUT: failed to wait for the properties, err: ", err)
	}
	roamTime := time.Since(startTime)
	s.Logf("DUT: roamed in %s", roamTime)
	if roamTime > param.expectedRoamTime {
		s.Fatalf("DUT: took to long to roam, expected to roam in %s", param.expectedRoamTime)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return tf.VerifyConnectionFromDUT(ctx, wificell.DefaultDUT, ap2)
	}, &testing.PollOptions{
		Timeout:  20 * time.Second,
		Interval: time.Second,
	}); err != nil {
		s.Fatal("Failed to verify connection: ", err)
	}
}
