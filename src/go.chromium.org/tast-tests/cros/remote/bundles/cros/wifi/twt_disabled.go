// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"net"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TwtDisabled,
		Desc: "Verifies TWT Requester Support is disabled in client HE MAC Capabilities and TWT Responder is enabled in AP HE MAC Capabilities",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func", "group:release-health", "release-health_wifi"},
		SoftwareDeps:    []string{"wifi"},
		HardwareDeps:    hwdep.D(hwdep.WifiIntel(), hwdep.Wifi80211ax()),
		ServiceDeps:     []string{wificell.ShillServiceName},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking, "wifi_router_features:WIFI_ROUTER_FEATURE_IEEE_802_11_AX"},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements:    []string{tdreq.WiFiGenSupport80211ax, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func TwtDisabled(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	// Get the MAC address of WiFi interface.
	iface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get WiFi interface of DUT: ", err)
	}
	ipr := ip.NewRemoteRunner(s.DUT().Conn())
	mac, err := ipr.MAC(ctx, iface)
	if err != nil {
		s.Fatal("Failed to get MAC of WiFi interface: ", err)
	}

	apOps := []hostapd.Option{
		hostapd.Mode(hostapd.Mode80211axPure),
		hostapd.Channel(40),
		hostapd.HTCaps(hostapd.HTCapHT20),
		hostapd.HEChWidth(hostapd.HEChWidth20Or40),
	}
	pcapPath, apConf, err := wifiutil.ConnectAndCollectPcap(ctx, tf, apOps)
	if err != nil {
		s.Fatal("Failed to collect packet: ", err)
	}

	// Get AP's BSSID
	apBSSID, err := net.ParseMAC(apConf.BSSID)
	if err != nil {
		s.Fatal("Failed to parse AP BSSID: ", err)
	}

	s.Log("Start analyzing pcap")

	// Check association request packets from client
	assocReqFilters := []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.TransmitterAddress(mac),
		pcap.TypeFilter(layers.LayerTypeDot11MgmtAssociationReq, nil),
	}
	assocReqPackets, err := pcap.ReadPackets(pcapPath, assocReqFilters...)
	if err != nil {
		s.Fatal("Failed to read association request packets: ", err)
	}
	s.Logf("Total %d assoc requests found", len(assocReqPackets))
	if len(assocReqPackets) == 0 {
		s.Fatal("No association request packets found")
	}

	// Check beacon and probe response packets from AP
	beaconProbeReqFilters := []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.TransmitterAddress(apBSSID),
		pcap.AnyOfTypesFilter([]gopacket.LayerType{layers.LayerTypeDot11MgmtProbeResp, layers.LayerTypeDot11MgmtBeacon}, nil),
	}
	beaconProbeReqPackets, err := pcap.ReadPackets(pcapPath, beaconProbeReqFilters...)
	if err != nil {
		s.Fatal("Failed to read beacon/probe response packets: ", err)
	}
	s.Logf("Total %d beacon/probe response packets found", len(beaconProbeReqPackets))
	if len(beaconProbeReqPackets) == 0 {
		s.Fatal("No beacon or probe response packets found")
	}

	checkHeIE := func(p gopacket.Packet) error {
		containsHE := false
		for _, l := range p.Layers() {
			element, ok := l.(*layers.Dot11InformationElement)
			if !ok {
				continue
			}
			// Ext Tag: HE capability
			if element.ID == 0xFF && element.Info[0] == 0x23 {
				containsHE = true
				// Check TWT Requester capability (bit 1 in HE MAC Capabilities)
				if (element.Info[1] & 0x02) == 0 {
					s.Log("TWT Requester disabled in HE Cap")
				} else {
					return errors.New("TWT Requester enabled in HE Cap")
				}
			}
			// Extended Capabilities
			if element.ID == 0x7F {
				if int(element.Length) >= 10 && (element.Info[9]&0x20) != 0 {
					return errors.New("TWT Requester enabled in Ext Cap")
				}
				s.Log("TWT Requester disabled in Ext Cap")
			}

		}
		if !containsHE {
			return errors.New("HE Capabilities IE missing")
		}
		return nil
	}

	checkAPHeIE := func(p gopacket.Packet) error {
		containsHE := false
		for _, l := range p.Layers() {
			element, ok := l.(*layers.Dot11InformationElement)
			if !ok {
				continue
			}
			// Ext Tag: HE capability
			if element.ID == 0xFF && element.Info[0] == 0x23 {
				containsHE = true
				// Check TWT Responder capability (bit 2 in HE MAC Capabilities)
				if (element.Info[1] & 0x04) != 0 {
					s.Log("TWT Responder enabled in AP HE Cap")
				} else {
					return errors.New("TWT Responder disabled in AP HE Cap")
				}
			}
		}
		if !containsHE {
			return errors.New("HE Capabilities IE missing in AP packets")
		}
		return nil
	}

	s.Log("Checking assoc request packets")
	for _, p := range assocReqPackets {
		if err := checkHeIE(p); err != nil {
			s.Fatal("Assoc request parsing failed: ", err)
		}
	}

	s.Log("Checking AP beacon/probe response packets")
	for _, p := range beaconProbeReqPackets {
		if err := checkAPHeIE(p); err != nil {
			s.Fatal("AP packet parsing failed: ", err)
		}
	}
}
