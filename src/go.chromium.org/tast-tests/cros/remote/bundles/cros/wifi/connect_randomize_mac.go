// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"net"

	cip "go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectRandomizeMAC,
		Desc: "Verifies that during connection the MAC address is randomized (or not) according to the setting",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		HardwareDeps: hwdep.D(hwdep.WifiMACAddrRandomize()),
		Requirements: []string{tdreq.WiFiGenSupportMARConn},
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnWifiDevice(wlan.Intel9260, wlan.Intel9000)),
			},
			// TODO(b/289148225): The chips {JfP2 and ThP2} send calibration frames with non-persistent MAC before scanning
			// when wpa_supplicant P2P flag is enabled which causes the test to fail. Remove the subtest unstable once
			// either the test is updated to ignore these frames or the Intel driver/fw stop sending them.
			{
				// This variant runs on unstable chipsets.
				Name:              "unstable",
				ExtraAttr:         []string{"wificell_unstable"},
				ExtraHardwareDeps: hwdep.D(hwdep.WifiDevice(wlan.Intel9260, wlan.Intel9000)),
			},
		},
	})
}

func ConnectRandomizeMAC(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	// Use 2.4GHz channel 1 for AP1.
	apOps := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.Channel(1),
		hostapd.HTCaps(hostapd.HTCapHT20),
	}
	ap1, err := tf.ConfigureAP(ctx, apOps, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanUpCtx := ctx
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap1)
	defer cancel()
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap1); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(cleanUpCtx)

	// We want control over capturer start/stop so we don't use fixture with
	// pcap but spawn it here and use manually.
	pcapRouter := tf.PcapRouter()

	// Get the MAC address of WiFi interface.
	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get WiFi interface of DUT: ", err)
	}
	ipr := ip.NewRemoteRunner(s.DUT().Conn())
	hwMAC, err := ipr.MAC(ctx, iface)
	if err != nil {
		s.Fatal("Failed to get MAC of WiFi interface: ", err)
	}
	s.Log("Read HW MAC: ", hwMAC)
	defer func(ctx context.Context, iface string, mac net.HardwareAddr) {
		if err := ipr.SetLinkDown(ctx, iface); err != nil {
			s.Error("Failed to set the interface down: ", err)
		}
		if err := ipr.SetMAC(ctx, iface, mac); err != nil {
			s.Error("Failed to revert the original MAC: ", err)
		}
		if err := ipr.SetLinkUp(ctx, iface); err != nil {
			s.Error("Failed to set the interface up: ", err)
		}
	}(ctx, iface, hwMAC)
	// Make sure the device is up.
	link, err := ipr.State(ctx, iface)
	if err != nil {
		s.Fatal("Failed to get link state")
	}
	if link != cip.LinkStateUp {
		if err := ipr.SetLinkUp(ctx, iface); err != nil {
			s.Error("Failed to set the interface up: ", err)
		}
	}

	// Routine to connect with PersistentRandom policy and get current MAC address together with captured packets.
	connectAndGetConnData := func(ctx context.Context, ap *wificell.APIface, name string) (macAddr net.HardwareAddr, pcapPath, servicePath string) {
		freqOps, err := ap.Config().PcapFreqOptions()
		if err != nil {
			s.Fatal("Failed to get frequency options for PcapRouter: ", err)
		}
		action := func(ctx context.Context) error {
			configProps := map[string]interface{}{
				shillconst.ServicePropertyWiFiRandomMACPolicy: shillconst.MacPolicyPersistentRandom,
			}
			resp, err := tf.ConnectWifiAP(ctx, ap, dutcfg.ConnProperties(configProps))
			if err != nil {
				return errors.Wrap(err, "failed to connect to WiFi")
			}
			servicePath = resp.ServicePath
			cleanUpCtx := ctx
			ctx, cancel := tf.ReserveForDisconnect(ctx)
			defer cancel()
			defer func(ctx context.Context) {
				if err := tf.DisconnectWifi(ctx); err != nil {
					testing.ContextLog(ctx, "Failed to disconnect WiFi: ", err)
				}
			}(cleanUpCtx)
			testing.ContextLog(ctx, "Connected to service: ", servicePath)
			macAddr, err = ipr.MAC(ctx, iface)
			if err != nil {
				return errors.Wrap(err, "failed to get MAC of WiFi interface")
			}
			return nil
		}
		pcapPath, err = wifiutil.CollectPcapForAction(ctx, pcapRouter, name, ap.Config().Channel, freqOps, action)
		if err != nil {
			s.Fatal("Failed to get packet capture path: ", err)
		}
		return macAddr, pcapPath, servicePath
	}

	// Connect to AP1 and check that MAC has changed.
	connMac, ap1pcap1, servicePath := connectAndGetConnData(ctx, ap1, "ap1-connect")
	s.Log("MAC after connection: ", connMac)
	if err := wifiutil.VerifyMACIsChanged(ctx, connMac, ap1pcap1, []net.HardwareAddr{hwMAC}); err != nil {
		s.Fatal("Failed to randomize MAC during connection: ", err)
	}

	// Reconnect to the same network and check that MAC is kept the same.
	reconnMac, ap1pcap2, _ := connectAndGetConnData(ctx, ap1, "ap1-reconnect")
	s.Log("MAC after re-connection: ", reconnMac)
	if err := wifiutil.VerifyMACIsKept(ctx, reconnMac, ap1pcap2, connMac); err != nil {
		s.Fatal("Failed to keep the MAC during re-connection: ", err)
	}

	// Switch to AP2 (also with randomization turned on) and check
	// that MAC has changed.  For AP2 use 5GHz channel 48.
	apOps[1] = hostapd.Channel(48)
	ap2, err := tf.ConfigureAP(ctx, apOps, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanUpCtx = ctx
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap2)
	defer cancel()
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap2); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(cleanUpCtx)

	connMac2, ap2pcap, servicePath2 := connectAndGetConnData(ctx, ap2, "ap2-connect")
	s.Log("MAC after connection to AP2: ", connMac2)
	if servicePath == servicePath2 {
		s.Fatal("The same service used for both AP1 and AP2: ", servicePath)
	}
	// This should be a new address - no previous one should be used.
	if err := wifiutil.VerifyMACIsChanged(ctx, connMac2, ap2pcap, []net.HardwareAddr{hwMAC, connMac}); err != nil {
		s.Fatal("Failed to change MAC for AP2: ", err)
	}

	// Go back to AP1 and check if we still have the same MAC as
	// before (we are using the PersistentRandom policy).
	connMac1, ap1pcap3, servicePath1 := connectAndGetConnData(ctx, ap1, "ap1-return")
	if servicePath1 != servicePath {
		s.Fatalf("Different service used during reconnection for AP1: got %s, want %s", servicePath1, servicePath)
	}
	s.Log("MAC after going back to AP1: ", connMac1)
	if err := wifiutil.VerifyMACIsKept(ctx, connMac1, ap1pcap3, connMac); err != nil {
		s.Fatal("Failed to keep the MAC for AP1 after switching back: ", err)
	}

	// Check that randomization for scans still works as expected.
	err = wifiutil.VerifyMACUsedForScan(ctx, tf, ap1, "disconnected-randomized", true,
		[]net.HardwareAddr{hwMAC, connMac1, connMac2})
	if err != nil {
		s.Fatal("Failed to verify correct MAC used during scanning: ", err)
	}

	s.Log("Completed successfully, cleaning up")
}
