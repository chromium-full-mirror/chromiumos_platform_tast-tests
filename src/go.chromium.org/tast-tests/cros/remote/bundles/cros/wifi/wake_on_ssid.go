// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WakeOnSSID,
		Desc: "Verifies that the DUT can wake up when a known SSID is discovered",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// This test is currently ungrouped due to lack of WakeOnWifi feature support.
		// Once this feature is implemented, please add the  attributes:
		// "wificell_func", "wificell_suspend", and "wificell_unstable"
		Attr: []string{"group:wificell"},
		// TODO(b/187362093): Add a SoftwareDep for wake_on_wifi.
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements:    []string{tdreq.WiFiPwrTimingWoW},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func WakeOnSSID(ctx context.Context, s *testing.State) {
	const (
		netDetectScanPeriod = 15 // In seconds.
	)

	tf := s.FixtValue().(*wificell.TestFixture)

	features := shillconst.WakeOnWiFiFeaturesDarkConnect
	wakeOnWifiOps := []wificell.SetWakeOnWifiOption{
		wificell.WakeOnWifiFeatures(features),
		wificell.WakeOnWifiNetDetectScanPeriod(netDetectScanPeriod),
	}
	ctx, restoreWakeOnWiFi, err := tf.WifiClient().SetWakeOnWifi(ctx, wakeOnWifiOps...)
	if err != nil {
		s.Fatal("Failed to set up wake on WiFi: ", err)
	}
	defer func() {
		if err := restoreWakeOnWiFi(); err != nil {
			s.Error("Failed to restore wake on WiFi setting: ", err)
		}
	}()

	// Set up the AP and connect.
	apOpts := []hostapd.Option{
		hostapd.SSID(hostapd.RandomSSID("TAST_TEST_WakeOnSSID")),
		hostapd.Mode(hostapd.Mode80211acMixed),
		hostapd.Channel(48),
		hostapd.HTCaps(hostapd.HTCapHT40),
		hostapd.VHTChWidth(hostapd.VHTChWidth20Or40),
	}

	ap, err := tf.ConfigureAP(ctx, apOpts, nil)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	defer func(ctx context.Context) {
		if ap == nil {
			// Already de-configured.
			return
		}
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()
	s.Log("AP setup done")

	connectResp, err := tf.ConnectWifiAP(ctx, ap)
	if err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	servicePath := connectResp.ServicePath
	defer func(ctx context.Context) {
		// If AP is already deconfigured, let's just wait for service idle.
		// Otherwise, explicitly disconnect.
		if ap == nil {
			if err := wifiutil.WaitServiceIdle(ctx, tf, servicePath); err != nil {
				s.Error("Failed to wait for DUT disconnected: ", err)
			}
		} else if err := tf.DisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect: ", err)
		}
	}(ctx)
	// This is over-reserving time, but for safety.
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	ctx, cancel = wifiutil.ReserveForWaitServiceIdle(ctx)
	defer cancel()
	s.Log("Connected")

	// Now deconfig AP so DUT is not connected before suspend.
	if err := tf.DeconfigAP(ctx, ap); err != nil {
		s.Fatal("Failed to deconfig AP: ", err)
	}
	ap = nil
	if err := wifiutil.WaitServiceIdle(ctx, tf, servicePath); err != nil {
		s.Fatal("Failed to wait for DUT disconnected: ", err)
	}
	s.Log("Disconnected")

	triggerFunc := func(ctx context.Context) error {
		// Re-spawn the AP. Notice the scope of "ap" so that the defer above can
		// clean it up.
		ap, err = tf.ConfigureAP(ctx, apOpts, nil)
		if err != nil {
			return errors.Wrap(err, "failed to configure AP")
		}
		return nil
	}
	// Expect the DUT can discover the service in one scan, so 2*netDetectScanPeriod should
	// be enough.
	if err := wifiutil.VerifyWakeOnWifiReason(ctx, tf, s.DUT(), 2*netDetectScanPeriod*time.Second,
		shillconst.WakeOnWiFiReasonSSID, triggerFunc); err != nil {
		s.Fatal("Verification failed: ", err)
	}

	// Spawn watcher to verify the DUT can get reconnected after woke up by SSID.
	// Use CHECK_WAIT here for a better timeout control.
	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	props := []*wificell.ShillProperty{
		{
			Property:       shillconst.ServicePropertyIsConnected,
			ExpectedValues: []interface{}{true},
			Method:         wifi.ExpectShillPropertyRequest_CHECK_WAIT,
		},
	}
	waitForProps, err := tf.WifiClient().ExpectShillProperty(waitCtx, servicePath, props, nil)
	if err != nil {
		s.Fatal("Failed to create a property watcher, err: ", err)
	}
	// Wait for the DUT to reconnect.
	if _, err := waitForProps(); err != nil {
		s.Fatal("Failed to wait for DUT to reconnect, err: ", err)
	}

	if err := tf.VerifyConnection(ctx, ap); err != nil {
		s.Fatal("Failed to verify WiFi connection after wakeup: ", err)
	}
}
