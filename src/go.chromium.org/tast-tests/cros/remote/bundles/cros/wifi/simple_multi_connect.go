// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SimpleMultiConnect,
		Desc: "Verifies that 2+ DUTs can connect to a single AP in default WiFi configuration",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		//TODO(b/304830047): Promote test to stable by removing wificell_cross_device_unstable tag.
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_multidut", "wificell_cross_device_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.PeripheralWifiStateWorking},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
	})
}

func SimpleMultiConnect(ctx context.Context, s *testing.State) {
	/*
		This test verifies that 2 DUTs can connect to a single AP in default WiFi configuration by:

		1. Setting up the AP with the DefaultOpenNetworkAP configuration.
		2. Connects the first DUT to the AP.
		3. Verifies the connection.
		3-1. Pings AP from DUT.
		3-2. Checks that no disconnect occurred during ping.
		4. Connects second DUT to AP.
		5. Verifies the connection (same as step 3).
		6. Disconnects the second DUT.
		7. Verifies the first DUT is still connected (same as step 3).
		8. Disconnects the first DUT.
		9. Deconfigures the AP.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	if tf.NumberOfDUTs() < 2 {
		s.Fatal("Test takes at least 2 DUTs. You Specified ", tf.NumberOfDUTs())
	}

	apIface, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}

	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, apIface); err != nil {
			s.Error("Failed to deconfig ap, err: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, apIface)
	defer cancel()
	s.Log("AP setup done")

	for n := 0; n < tf.NumberOfDUTs(); n++ {

		dutID := wificell.DutIdx(n)

		s.Log("Attempting to connect DUT #", dutID)

		_, err := tf.ConnectWifiAPFromDUT(ctx, dutID, apIface)
		if err != nil {
			s.Fatal("Failed to connect to WiFi, err: ", err)
		}
		defer func(ctx context.Context, idx wificell.DutIdx) {
			s.Logf("Disconnecting DUT #%d", idx)
			if err := tf.DisconnectDUTFromWifi(ctx, idx); err != nil {
				s.Error("Failed to disconnect WiFi, err: ", err)
			}
		}(ctx, dutID)

		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		s.Log("Connected")

		verifyConnection := func(ctx context.Context, idx wificell.DutIdx) {
			ping := func(ctx context.Context) error {
				_, err := tf.PingFromSpecificDUT(ctx, idx, apIface.ServerIP().String())
				return err
			}

			if err := tf.AssertNoDisconnect(ctx, idx, ping); err != nil {
				s.Fatal("Failed to ping from DUT, err: ", err)
			}
		}

		verifyConnection(ctx, dutID)

		defer func(ctx context.Context, idx wificell.DutIdx) {
			s.Logf("Verifying DUT #%d is still connected", idx)
			verifyConnection(ctx, dutID)
		}(ctx, dutID)

		s.Log("Deconfiguring")
	}

	s.Log("Tearing down")
}
