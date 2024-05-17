// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/google/gopacket/layers"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// scanAndConnect6GHzTestCase holds parameters of a ScanAndConnect6GHz test variant.
type scanAndConnect6GHzTestCase struct {
	// isOutOfBand indicates if it is out-of-band discovery
	isOutOfBand bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ScanAndConnect6GHz,
		Desc: "Verifies that the DUT supports both in-band and out-of-band discovery",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
			"kaidong@google.com",              // Test author
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func", "wificell_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking, "wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX_E"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
		},
		HardwareDeps: hwdep.D(hwdep.Wifi80211ax6E()),
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{
			{
				Name: "in_band",
				Val: scanAndConnect6GHzTestCase{
					isOutOfBand: false,
				},
			},
			{
				Name: "out_of_band",
				Val: scanAndConnect6GHzTestCase{
					isOutOfBand: true,
				},
			},
		},
	})
}

func ScanAndConnect6GHz(ctx context.Context, s *testing.State) {
	/*
		This test verifies WiFi 6E devices supports both in-band and out-of-band discovery.
		1. Turn off the background scan on the DUT
		2. Turn off MAC randomization on the DUT
		3. Set WiFi RequestScan type on the DUT
		4. Set up the APs. The 5 GHz AP is enabled only if the 6 GHz AP is co-located
		5. Connect the DUT to the 6 GHz AP
		6. Verify the RequestScan type is as expected by checking probe requests in the pcap file
	*/

	const (
		apChannel = 193
		opClass   = 131
	)
	tf := s.FixtValue().(*wificell.TestFixture)
	tc := s.Param().(scanAndConnect6GHzTestCase)
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Unable to get client interface name: ", err)
	}
	ipr := ip.NewRemoteRunner(s.DUT().Conn())
	dutMAC, err := ipr.MAC(ctx, clientIface)
	if err != nil {
		s.Fatal("Failed to get MAC of WiFi interface: ", err)
	}

	// Turn off MAC randomization for scans, so that MAC address used for scan is not randomized,
	// the probe requests from the DUT can be identified by its MAC address
	resp, err := tf.WifiClient().SetMACRandomize(ctx, &wifi.SetMACRandomizeRequest{Enable: false})
	if err != nil {
		s.Fatal("Failed to turn off MAC randomization: ", err)
	}
	if resp.OldSetting {
		testing.ContextLog(ctx, "Switched MAC randomization for scans to false")
		// Restore the setting on leaving.
		defer func(ctx context.Context) {
			if _, err := tf.WifiClient().SetMACRandomize(ctx, &wifi.SetMACRandomizeRequest{Enable: true}); err != nil {
				s.Fatal("Failed to restore MAC randomization setting back: ", err)
			}
		}(ctx)
	}

	// Turn off background and foreground scans, so that all the WiFi scans are triggered by shill::Manager::RequestScan()
	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	capturer, err := func(ctx context.Context) (*pcap.Capturer, error) {
		// Set WiFi request scan type on the DUT
		var requestScanType string
		if tc.isOutOfBand {
			requestScanType = shillconst.WiFiRequestScanTypeActive
		} else {
			requestScanType = shillconst.WiFiRequestScanTypePassive
		}
		originalRequestScanType, err := tf.WifiClient().GetRequestScanTypeProperty(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get WiFi RequestScan type")
		}
		if originalRequestScanType != requestScanType {
			defer func(ctx context.Context) {
				if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, originalRequestScanType); err != nil {
					s.Errorf("Failed to reset WiFi RequestScan type to %s: %v", originalRequestScanType, err)
				}
				s.Log("Reset WiFi RequestScan type to ", originalRequestScanType)
			}(ctx)
			ctx, cancel = ctxutil.Shorten(ctx, 500*time.Millisecond)
			defer cancel()
			if err := tf.WifiClient().SetRequestScanTypeProperty(ctx, requestScanType); err != nil {
				s.Fatalf("Failed to set WiFi RequestScan type to %s: %v", requestScanType, err)
			}
			s.Log("Set WiFi RequestScan type to ", requestScanType)
		}

		ap6GHzOpts := []hostapd.Option{hostapd.SSID(hostapd.RandomSSID("6GHz_")), hostapd.Mode(hostapd.Mode80211axPure),
			hostapd.HTCaps(hostapd.HTCapHT20), hostapd.HEChWidth(hostapd.HEChWidth20Or40), hostapd.OpClass(opClass),
			hostapd.Channel(apChannel), hostapd.SpectrumManagement(), hostapd.PMF(hostapd.PMFRequired)}
		secConfFac := wpa.NewConfigFactory("chromeos",
			wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP))
		var apConfigs []hostapd.ApConfig
		if tc.isOutOfBand {
			// Set up the 5 GHz AP for an out-of-band discovery, i.e., the 6 GHz AP is co-located
			ap5GHzOpts := []hostapd.Option{hostapd.SSID(hostapd.RandomSSID("5GHz_")), hostapd.Mode(hostapd.Mode80211acPure),
				hostapd.HTCaps(hostapd.HTCapHT20), hostapd.OpClass(0), hostapd.Channel(40), hostapd.SpectrumManagement()}
			apConfigs = append(apConfigs, hostapd.ApConfig{ApOpts: ap6GHzOpts, SecConfFac: secConfFac})
			apConfigs = append(apConfigs, hostapd.ApConfig{ApOpts: ap5GHzOpts, SecConfFac: nil})
		} else {
			// Intel driver doesn't support FILS Discovery frame or unsolicited broadcast Probe Response frame transmission,
			// set beacon interval to 20 TUs so that the DUT receives beacon frames within the channel dwell time
			ap6GHzOpts = append(ap6GHzOpts, hostapd.BeaconInterval(20))
			apConfigs = append(apConfigs, hostapd.ApConfig{ApOpts: ap6GHzOpts, SecConfFac: secConfFac})
		}
		ap, err := tf.ConfigureAPOnRouterIDWithConfs(ctx, 0, apConfigs, "", true, false, false)
		if err != nil {
			return nil, errors.Wrap(err, "failed to configure the APs")
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, ap); err != nil {
				s.Error("Failed to deconfig the APs: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		var conf *hostapd.Config
		for _, conf = range ap.Configs() {
			if conf.OpClass == opClass && conf.Channel == apChannel {
				break
			}
		}
		if conf == nil {
			s.Fatal("Failed to get the Config of the 6 GHz AP")
		}
		_, err = tf.ConnectWifiFromDUT(ctx, wificell.DefaultDUT, conf.SSID, dutcfg.ConnSecurity(conf.SecurityConfig))
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to WiFi")
		}
		defer func(ctx context.Context) {
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		s.Log("Connected to the 6 GHz AP")
		// The AP may have two capturers for both 5 GHz and 6 GHz BSSs
		capturers, ok := tf.Capturers(ap)
		if !ok {
			return nil, errors.Wrap(err, "failed to get the capturers")
		}
		freq, err := hostapd.ChannelToFrequencyWithOpClass(apChannel, opClass)
		if err != nil {
			s.Fatal(err, "Failed to get the frequency of the 6 GHz AP")
		}
		capturer, ok := capturers[freq]
		if !ok {
			return nil, errors.Wrapf(err, "failed to get the capturer of frequency %d", freq)
		}
		return capturer, nil
	}(ctx)
	if err != nil {
		s.Fatal("Failed to scan and connect to the 6 GHz AP: ", err)
	}
	pcapPath, err := capturer.PacketPath(ctx)
	if err != nil {
		s.Fatal("Failed to get packet capture: ", err)
	}

	probeReqFilter := []pcap.Filter{
		pcap.TypeFilter(layers.LayerTypeDot11MgmtProbeReq, nil),
		pcap.TransmitterAddress(dutMAC),
	}
	probeReqPackets, err := pcap.ReadPackets(pcapPath, probeReqFilter...)
	if err != nil {
		s.Fatal("Failed to read probe requests from packet capture: ", err)
	}
	if tc.isOutOfBand && len(probeReqPackets) == 0 {
		s.Fatal("The DUT did not perform active scan during out-of-band discovery")
	}
	if !tc.isOutOfBand && len(probeReqPackets) > 0 {
		s.Fatal("The DUT did not perform passive scan during in-band discovery")
	}
}
