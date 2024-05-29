// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RoamDbus,
		Desc: "Tests an intentional client-driven roam between APs",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func RoamDbus(ctx context.Context, s *testing.State) {
	/*
		This test checks a DUT's ability to force a roam between APs using the
		following steps:
		1 - Turn off foreground and background scans, and set ScanAllowRoam
		    shill property to false.
		2 - Configure AP1.
		3 - Connect DUT to AP1.
		4 - Configure AP2 on the same SSID as AP1.
		5 - Send dbus roam command on the DUT so that it roams to AP2.
		6 - Verify the DUT roams to the AP2 in a reasonable amount of time.
		7 - Verify there were no disconnections during the roam.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	// Configure AP1 and connect the DUT to it, then configure AP2.
	ap1Config := hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(48), hostapd.HTCaps(hostapd.HTCapHT20)}}
	ap2Config := hostapd.ApConfig{ApOpts: []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20)}}
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
	rt.SetRoamSucceeded(false)

	// Generate a property watcher for changes in roam state and WiFi.BSSID.
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	waitForProps, err := tf.DUTWifiClient(wificell.DefaultDUT).GenerateRoamPropertyWatcher(waitCtx, rt.AP2BSSID(), rt.ServicePath())
	if err != nil {
		s.Fatal("DUT: failed to create a property watcher, err: ", err)
	}

	// Send a scan-only request to shill so that the DUT discovers AP2BSSID.
	iface, err := tf.DUTClientInterface(ctx, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("DUT: ", err)
	}
	discCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := tf.DUTWifiClient(wificell.DefaultDUT).DiscoverBSSID(discCtx, rt.AP2BSSID(), iface, []byte(rt.AP1SSID())); err != nil {
		s.Fatalf("DUT: failed to find the BSSID %s: %v", rt.AP2BSSID(), err)
	}

	// Send roam command to shill, and shill will send D-Bus roam command to wpa_supplicant.
	s.Logf("Requesting roam from %s to %s", rt.AP2BSSID(), rt.AP2BSSID())
	if err := tf.DUTWifiClient(wificell.DefaultDUT).RequestRoam(ctx, iface, rt.AP2BSSID(), 30*time.Second); err != nil {
		s.Errorf("DUT: failed to roam from %s to %s: %v", rt.AP2BSSID(), rt.AP2BSSID(), err)
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
