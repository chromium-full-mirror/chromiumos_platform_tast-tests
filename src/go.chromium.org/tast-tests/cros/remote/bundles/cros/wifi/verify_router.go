// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"slices"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	remoteiw "go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	signalThreshold   = -60
	varianceThreshold = 15
)

type verifyRouterParams struct {
	name  string
	apOps []hostapd.Option
}

func init() {
	testing.AddTest(&testing.Test{
		Func: VerifyRouter,
		Desc: "Verifies router functionality with multiple APs by checking signal strength and variance",
		Contacts: []string{
			"chromeos-wifi-champs@google.com",
			"justin.lee@cienet.com",
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Requirements:    []string{tdreq.WiFiGenSupportWiFi},
		Timeout:         time.Minute * 10,
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name: "2g",
				Val: verifyRouterParams{
					name: "2G",
					apOps: []hostapd.Option{
						hostapd.Mode(hostapd.Mode80211nPure),
						hostapd.Channel(6),
						hostapd.HTCaps(hostapd.HTCapHT40),
					},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCapture),
			},
			{
				Name: "5g",
				Val: verifyRouterParams{
					name: "5G",
					apOps: []hostapd.Option{
						hostapd.Mode(hostapd.Mode80211acPure),
						hostapd.Channel(48),
						hostapd.HTCaps(hostapd.HTCapHT40),
						hostapd.VHTChWidth(hostapd.VHTChWidth20Or40),
					},
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesCapture),
			},
		},
	})
}

// VerifyRouter test to verify router functionality with multiple APs.
func VerifyRouter(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	params := s.Param().(verifyRouterParams)

	cleanupCtx, cancel := tf.ReserveForDisconnect(ctx)
	defer cancel()

	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get client interface: ", err)
	}

	dutiwr := remoteiw.NewRemoteRunner(s.DUT().Conn())

	var aps []*wificell.APIface
	for i := 0; i < 2; i++ {
		ap, err := tf.ConfigureAP(ctx, params.apOps, nil)
		if err != nil {
			s.Fatalf("Failed to configure AP%d: %v", i+1, err)
		}
		aps = append(aps, ap)
	}
	defer tf.DeconfigAllAPs(cleanupCtx)

	for i, ap := range aps {
		if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
			s.Fatalf("Failed to connect to AP%d: %v", i+1, err)
		}

		if err := verifySignalLevels(ctx, dutiwr, clientIface); err != nil {
			s.Fatalf("Signal check failed for AP%d: %v", i+1, err)
		}

		// Avoid automatically reconnecting to the AP.
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Fatal("Failed to disconnect: ", err)
		}
	}
}

// verifySignalLevels checks the signal strength on the DUT against thresholds.
func verifySignalLevels(ctx context.Context, iwr *remoteiw.Runner, clientIface string) error {
	signalLevels, err := iwr.WifiInterfaceSignalLevelAllChains(ctx, clientIface)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve chain signal info from device")
	}

	if len(signalLevels) == 0 {
		return errors.New("no chain signals found")
	}

	maxSignal := slices.Max(signalLevels)
	minSignal := slices.Min(signalLevels)

	if minSignal < signalThreshold {
		return errors.Errorf("signal too weak on at least one antenna (%v dBm)", signalLevels)
	}
	if maxSignal-minSignal > varianceThreshold {
		return errors.Errorf("Antenna signals vary significantly (%v dBm)", signalLevels)
	}

	return nil
}
