// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type roamTo6GHzTestCase struct {
	lowerBandChannel int
}

func init() {
	testing.AddTest(&testing.Test{
		Func: RoamTo6GHz,
		Desc: "Tests an natural roam to a 6GHz AP",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"rmekonnen@google.com",            // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// TODO(b/333388420) Stabilize RoamTo6GHz test
		Attr:            []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking, "wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E"},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		SoftwareDeps:    []string{"wpa3_sae"},
		HardwareDeps:    hwdep.D(hwdep.Wifi80211ax6E()),
		Params: []testing.Param{
			{
				Name: "from_2ghz",
				Val: roamTo6GHzTestCase{
					lowerBandChannel: 1,
				},
			}, {
				Name: "from_5ghz",
				Val: roamTo6GHzTestCase{
					lowerBandChannel: 40,
				},
			},
		},
	})
}

func RoamTo6GHz(ctx context.Context, s *testing.State) {
	/*
		This test checks a DUT's ability to naturally roam to a 6GHz AP by using
		the following steps:
		1 - Turn off foreground and background scans, and set ScanAllowRoam
		    shill property to true.
		2 - Configure legacy band AP.
		3 - Associate DUT to legacy band AP.
		4 - Configure the 6GHz AP with the same SSID as legacy band AP.
		5 - Set the DUT RequestScanType to active.
		6 - Request a scan.
		7 - Verify the DUT roams to the 6GHz AP in a reasonable amount of time.
		8 - Verify there were no disconnections during the roam.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(roamTo6GHzTestCase)
	lowerBandApConfig := hostapd.ApConfig{
		ApOpts:     []hostapd.Option{hostapd.Mode(hostapd.Mode80211axMixed), hostapd.Channel(tc.lowerBandChannel), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.SpectrumManagement()},
		SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP))}
	higherBandApConfig := hostapd.ApConfig{
		ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211axPure), hostapd.Channel(21), hostapd.HTCaps(hostapd.HTCapHT20),
			hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.OpClass(131), hostapd.PMF(hostapd.PMFRequired), hostapd.BeaconInterval(20)},
		SecConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP))}

	// Configure the legacy band AP and connect the DUT to it, then configure
	// the 6GHz band AP.
	ctx, rt, finish, err := wifiutil.SimpleRoamInitialSetup(ctx, tf, wificell.DefaultDUT, lowerBandApConfig, higherBandApConfig, true)
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
	rt.SetRoamSucceeded(false)

	// Set WiFi RequestScanType on the DUT to active to ensure the 6GHz AP
	// is discovered in a timely manner.
	originalRequestScanType, err := tf.WifiClient().GetRequestScanTypeProperty(ctx)
	if err != nil {
		s.Error("Failed to get WiFi RequestScan type: ", err)
	}
	if originalRequestScanType != shillconst.WiFiRequestScanTypeActive {
		defer func(ctx context.Context) {
			if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, originalRequestScanType); err != nil {
				s.Errorf("Failed to reset WiFi RequestScan type to %s: %v", originalRequestScanType, err)
			}
			s.Log("Reset WiFi RequestScan type to ", originalRequestScanType)
		}(ctx)
		ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
		defer cancel()
		if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, shillconst.WiFiRequestScanTypeActive); err != nil {
			s.Fatal("Failed to set WiFi RequestScan type to active: ", err)
		}
		s.Log("Set WiFi RequestScan type to active")
	}

	// Generate a property watcher for changes in roam state and WiFi.BSSID.
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	waitForProps, err := tf.DUTWifiClient(wificell.DefaultDUT).GenerateRoamPropertyWatcher(waitCtx, rt.AP2BSSID(), rt.ServicePath())
	if err != nil {
		s.Fatal("DUT: failed to create a property watcher, err: ", err)
	}

	// No need to request a roam to the 6GHz AP, the DUT should automatically
	// roam after a scan.
	if err := tf.DUTWifiClient(wificell.DefaultDUT).RequestScan(ctx); err != nil {
		s.Fatal("Failed to request scan: ", err)
	}

	detectTimeout := 10 * time.Second
	s.Log("Waiting for AP2 to be found")
	req := &wifi.WaitForBSSIDRequest{
		Ssid:  []byte(rt.AP1SSID()),
		Bssid: rt.AP2BSSID(),
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()
	if _, err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForBSSID(timeoutCtx, req); err != nil {
		s.Error("Failed to detect the 6GHz AP BSSID: ", err)
	}

	// Verify that the DUT has roamed successfully.
	monitorResult, err := waitForProps()
	if err != nil {
		s.Fatal("DUT: failed to wait for the properties, err: ", err)
	}
	s.Log("DUT: roamed")
	rt.SetRoamSucceeded(true)
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Error("Failed to disconnect WiFi: ", err)
		}
	}(ctx)

	// Assert there was no disconnection during roaming.
	if err := wifiutil.VerifyNoDisconnections(monitorResult); err != nil {
		s.Fatal("DUT: failed to stay connected during the roaming process: ", err)
	}

	if err := tf.VerifyConnection(ctx, rt.AP2()); err != nil {
		s.Fatal("DUT: failed to verify connection: ", err)
	}
}
