// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/framesender"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MalformedProbeResp,
		Desc: "Test that we can stay connected to the configured AP when receiving malformed probe responses from an AP that we are not connected to",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func MalformedProbeResp(ctx context.Context, s *testing.State) {
	// The test uses framesender to periodically send malformed probe
	// responses and trigger background scans on the same channel. Then
	// it verifies that the SSID of the malformed responses are found
	// and no disconnection during the period.
	tf := s.FixtValue().(*wificell.TestFixture)

	router, err := tf.StandardRouterWithFrameSenderSupport()
	if err != nil {
		s.Fatal("Failed to get legacy router: ", err)
	}

	// We'll use `iw scan` to trigger background scan, turn it off
	// in shill so we won't race with shill on the device.
	ctx, restoreBgAndFgscan, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFgscan(); err != nil {
			s.Error("Failed to restore background and/or foreground scan config: ", err)
		}
	}()

	s.Log("Configuring AP")
	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Deconfiguring AP")
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	s.Log("Connecting")
	if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Disconnecting")
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	// Get DUT interface name and MAC address.
	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get client interface: ", err)
	}
	ipr := ip.NewRemoteRunner(s.DUT().Conn())
	mac, err := ipr.MAC(ctx, iface)
	if err != nil {
		s.Fatal("Failed to get MAC of WiFi interface: ", err)
	}

	// Start the background sender of malformed probe response.
	sender, err := router.NewFrameSender(ctx, ap.Interface())
	if err != nil {
		s.Fatal("Failed to create frame sender: ", err)
	}
	defer func(ctx context.Context) {
		if err := router.CloseFrameSender(ctx, sender); err != nil {
			s.Error("Failed to close frame sender: ", err)
		}
	}(ctx)
	ctx, cancel = ctxutil.Shorten(ctx, common.RouterCloseFrameSenderDuration)
	defer cancel()

	// Set up background frame sender sending malformed probe response.
	const ssidPrefix = "TestingProbes"
	fsOps := []framesender.Option{
		framesender.SSIDPrefix(ssidPrefix),
		framesender.NumBSS(1),
		framesender.Count(0),  // Infinite run.
		framesender.Delay(50), // 50ms delay.
		framesender.DestMAC(mac.String()),
		// Append a vendor specific IE (0xdd) with broken length (0xb7 > the
		// length of remaining payload) and OUI=00:1a:11 which is Google.
		framesender.ProbeRespFooter([]byte("\xdd\xb7\x00\x1a\x11\x01\x01\x02\x03")),
	}
	if err = sender.Start(ctx, framesender.TypeProbeResponse, ap.Config().Channel, fsOps...); err != nil {
		s.Error("sender failed: ", err)
	}
	defer func(ctx context.Context) {
		if err := sender.Stop(ctx); err != nil {
			s.Error("Failed to stop frame sender: ", err)
		}
	}(ctx)
	ctx, cancel = sender.ReserveForStop(ctx)
	defer cancel()

	runOnce := func(ctx context.Context) (retErr error) {
		const scanLoopTime = 60 * time.Second
		const scanLoopInterval = 10 * time.Second

		start := time.Now()
		received := 0
		// Framesender fills in SSID with "0"s.
		ssid := ssidPrefix + "00000000"
		for round := 1; time.Since(start) < scanLoopTime; round++ {
			s.Logf("Scan %d", round)
			err = tf.DUTWifiClient(wificell.DefaultDUT).RequestScan(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to scan")
			}
			path, err := tf.DUTWifiClient(wificell.DefaultDUT).GetServicePath(ctx, map[string]interface{}{
				shillconst.ServicePropertyType:        shillconst.TypeWifi,
				shillconst.ServicePropertyWiFiHexSSID: strings.ToUpper(hex.EncodeToString([]byte(ssid))),
			})
			if err == nil {
				s.Logf("Found BSS %q in service: %v", ssid, path)
				received++
			}
			// GoBigSleepLint this sleep is the part of the test design.
			if err := testing.Sleep(ctx, scanLoopInterval); err != nil {
				return err
			}
		}
		if received == 0 {
			return errors.New("no probe response received")
		}

		return nil
	}

	if err := tf.AssertNoDisconnect(ctx, wificell.DefaultDUT, runOnce); err != nil {
		s.Fatal("Disconnection event found: ", err)
	}
}
