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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WakeOnDisconnect,
		Desc: "Verifies that the DUT can wake up when disconnected from AP",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// TODO(b/187362093): Add a SoftwareDep for wake_on_wifi.
		Attr:         []string{"group:wificell", "wificell_func", "wificell_suspend", "wificell_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiPwrTimingWoW},
	})
}

func WakeOnDisconnect(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	features := shillconst.WakeOnWiFiFeaturesDarkConnect
	ctx, restoreWakeOnWiFi, err := tf.WifiClient().SetWakeOnWifi(ctx, wificell.WakeOnWifiFeatures(features))
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
		hostapd.SSID(hostapd.RandomSSID("TAST_TEST_WakeOnDisconnect")),
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
	defer func(ctx context.Context) {
		// If AP is already deconfigured, let's just wait for service idle.
		// Otherwise, explicitly disconnect.
		if ap == nil {
			if err := wifiutil.WaitServiceIdle(ctx, tf, connectResp.ServicePath); err != nil {
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

	triggerFunc := func(ctx context.Context) error {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			return errors.Wrap(err, "failed to deconfig AP")
		}
		ap = nil
		return nil
	}
	// Assume DUT could notice AP gone in 10 seconds. (by deauth or missing beason)
	if err := wifiutil.VerifyWakeOnWifiReason(ctx, tf, s.DUT(), 10*time.Second,
		shillconst.WakeOnWiFiReasonDisconnect, triggerFunc); err != nil {
		s.Fatal("Verification failed: ", err)
	}
}
