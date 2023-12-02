// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Reset,
		Desc: "Test that the WiFi interface can be reset successfully, and that WiFi comes back up properly",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_suspend"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		// For some Marvell DUT, this test may take more than 25 minutes.
		// For WCN3990 device, this test may take more than 39 minutes.
		Timeout: time.Minute * 45,
		// We only support reset on Intel/Marvell/QCA/RTK/MTK WiFi (iwlwifi/mwifiex/ath10k/rtw88/mt76).
		// TODO(chromium:1070299): Currently we might need to exclude the unsupported devices with hwdep.SkipOnModel.
		// Replace them with more proper hwdep in the future.
		Params: []testing.Param{
			{
				// Default AP settings ported from Autotest.
				Val: []hostapd.Option{hostapd.Mode(hostapd.Mode80211b), hostapd.Channel(1)},
			},
			{
				// The target protocol and channel settings, as this is more widely used nowadays.
				// TODO(b/175602523): Replace the default with this once the issue is fixed.
				Name: "80211n_ch48",
				Val:  wificell.DefaultOpenNetworkAPOptions(),
			},
		},
	})
}

func Reset(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	var errReset error
	defer func(ctx context.Context) {
		if errReset != nil {
			// recovering from bad state
			if err := s.DUT().Reboot(ctx); err != nil {
				s.Fatal("Failed to reboot DUT: ", err)
			}
		}
	}(ctx)
	ctx, cancel := tf.ReserveForReboot(ctx)
	defer cancel()

	apOps := s.Param().([]hostapd.Option)
	ap, err := tf.ConfigureAP(ctx, apOps, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfigure the AP: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	ctxForDisconnectWiFi := ctx
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	resp, err := tf.ConnectWifiAP(ctx, ap)
	if err != nil {
		s.Fatal("Failed to connect to the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect from the AP: ", err)
		}
	}(ctxForDisconnectWiFi)

	if err := tf.PingFromDUT(ctx, ap.ServerIP().String()); err != nil {
		s.Fatal("Failed to ping from the DUT: ", err)
	}

	if _, errReset = tf.WifiClient().ResetTest(ctx, &wifi.ResetTestRequest{
		ServicePath: resp.ServicePath,
		ServerIp:    ap.ServerIP().String(),
	}); errReset != nil {
		s.Fatal("gRPC command ResetTest failed: ", errReset)
	}
}
