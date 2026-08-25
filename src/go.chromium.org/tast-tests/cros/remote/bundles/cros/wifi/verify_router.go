// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
	isConductive bool
}

type bandConfig struct {
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
		Attr:            []string{"group:wificell", "wificell_func", "group:bluetooth_wifi_testbed_update"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Requirements:    []string{tdreq.WiFiGenSupportWiFi},
		Timeout:         time.Minute * 10,
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name: "ota",
				Val: verifyRouterParams{
					isConductive: false,
				},
				Fixture: wificell.FixtureID(wificell.TFFeaturesRoutersWithoutBT),
			},
			{
				Name: "conductive",
				Val: verifyRouterParams{
					isConductive: true,
				},
				ExtraTestBedDeps: []string{tbdep.Conductive},
				Fixture:          wificell.FixtureID(wificell.TFFeaturesRoutersWithoutBT),
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

	bands := []bandConfig{
		{
			name: "2G",
			apOps: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.Channel(6),
				hostapd.HTCaps(hostapd.HTCapHT40),
			},
		},
		{
			name: "5G",
			apOps: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211acPure),
				hostapd.Channel(48),
				hostapd.HTCaps(hostapd.HTCapHT40),
				hostapd.VHTChWidth(hostapd.VHTChWidth20Or40),
			},
		},
	}

	// Pcap will act as the second router if provided.
	for rIdx := 0; rIdx < tf.ConfiguredRouterCount(); rIdx++ {
		routerIdx := wificell.RouterIdx(rIdx)
		routerTarget := tf.RouterByID(routerIdx)

		for _, band := range bands {
			func(ctx context.Context) {
				var aps []*wificell.APIface
				for i := 0; i < 2; i++ {
					ap, err := tf.ConfigureAPOnRouterID(ctx, routerIdx, band.apOps, nil, false, false)
					if err != nil {
						s.Fatalf("Failed to configure %s AP%d on Router%d: %v", band.name, i+1, rIdx+1, err)
					}
					aps = append(aps, ap)
				}
				defer tf.DeconfigAllAPs(cleanupCtx)

				for i, ap := range aps {
					s.Run(ctx, fmt.Sprintf("Router%d_AP%d_%s", rIdx+1, i+1, band.name), func(ctx context.Context, s *testing.State) {
						if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
							s.Fatal("Failed to connect: ", err)
						}

						if err := verifySignalLevels(ctx, dutiwr, clientIface, routerTarget.RouterModel(), params.isConductive); err != nil {
							s.Fatal("Signal check failed: ", err)
						}

						if err := tf.CleanDisconnectWifi(ctx); err != nil {
							s.Fatal("Failed to disconnect: ", err)
						}
					})
				}
			}(ctx)
		}
	}
}

// verifySignalLevels checks the signal strength on the DUT against thresholds.
func verifySignalLevels(ctx context.Context, iwr *remoteiw.Runner, clientIface, model string, isConductive bool) error {
	signalLevelStr, err := iwr.WifiInterfaceSignalLevel(ctx, clientIface)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve signal info from device")
	}
	signalLevel, err := strconv.Atoi(strings.TrimSpace(signalLevelStr))
	if err != nil {
		return errors.Wrap(err, "failed to parse signal level")
	}
	if signalLevel < signalThreshold {
		return errors.Errorf("signal too weak (%d dBm)", signalLevel)
	}

	// Skip chain variance check for OTA or Gale.
	if !isConductive || model == "gale" {
		return nil
	}

	signalLevels, err := iwr.WifiInterfaceSignalLevelAllChains(ctx, clientIface)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve chain signal info from device")
	}

	if len(signalLevels) == 0 {
		return errors.New("no chain signals found")
	}

	maxSignal := slices.Max(signalLevels)
	minSignal := slices.Min(signalLevels)

	if maxSignal-minSignal > varianceThreshold {
		return errors.Errorf("Antenna signals vary significantly (%v dBm)", signalLevels)
	}
	return nil
}
