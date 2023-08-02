// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"bytes"
	"context"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MBOAssocDisallow,
		Desc: "Verifies that a DUT won't connect to an AP with the assoc disallow bit set",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		SoftwareDeps: []string{"mbo"},
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
	})
}

func MBOAssocDisallow(ctx context.Context, s *testing.State) {
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

	// Turn off the background scan to avoid outgoing scan before setting AP.
	ctx, restoreBg, err := tf.WifiClient().TurnOffBgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background scan: ", err)
	}
	defer func() {
		if err := restoreBg(); err != nil {
			s.Error("Failed to restore the background scan config: ", err)
		}
	}()

	s.Log("Configuring AP")
	testSSID := hostapd.RandomSSID("MBO_ASSOC_DISALLOW_")
	channel := 1
	apOpts := []hostapd.Option{hostapd.SSID(testSSID), hostapd.Mode(hostapd.Mode80211nMixed), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.Channel(channel), hostapd.MBO()}
	ap, err := tf.ConfigureAP(ctx, apOpts, nil)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	freqOpts, err := ap.Config().PcapFreqOptions()
	if err != nil {
		s.Fatal("Failed to get pcap frequency options: ", err)
	}

	s.Log("Setting assoc disallow")
	if err := ap.Set(ctx, hostapd.PropertyMBOAssocDisallow, "1"); err != nil {
		s.Fatal("Unable to set assoc disallow on AP: ", err)
	}

	// Get the name of the DUT WiFi interface and flush BSS from WPA
	// supplicant to make sure it has seen assoc disallow bit.
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Unable to get DUT interface name: ", err)
	}
	s.Log("Flushing BSS cache")
	if err := tf.WifiClient().FlushBSS(ctx, clientIface, 0); err != nil {
		s.Fatal("Failed to flush BSS list: ", err)
	}

	// Make sure the DUT WiFi interface knows the MBO configuration by checking
	// probe response or beacon frames, which are two mgmt frames that propagate
	// this information. Probe response is triggered by active scans and
	// therefore both types of frames can be collected during scans.
	// Here use |scanNum| scans to make sure probe response is captured. This
	// number should be large enough so that there is sufficient time to capture
	// at least one targeted frame (AP may not set this value or propagate this
	// information immediately after configuration, and extra time is needed to
	// collect probe response after triggering scans) but cannot be too large to
	// compromise the efficiency.
	const scanNum = 3
	pcapPath, err := wifiutil.ScanAndCollectPcap(ctx, tf, "mbo_probresp_or_beacon", scanNum, channel)
	if err != nil {
		s.Fatal("Failed to collect packet: ", err)
	}
	s.Log("Start analyzing pcap on probe response or beacon frames")
	filters := []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.AnyOfTypesFilter([]gopacket.LayerType{layers.LayerTypeDot11MgmtProbeResp, layers.LayerTypeDot11MgmtBeacon}, nil),
	}
	pac, err := pcap.ReadPackets(pcapPath, filters...)
	numPac := len(pac)
	if numPac == 0 {
		s.Fatal("No probe response or beacon frames collected")
	}
	s.Logf("Checking packets: %d captured", numPac)

	checkAssocDisallowBit := func(p gopacket.Packet) error {
		// Traverse all IEs, find the MBO-ICE tag and check if assocDisallow is set.
		for _, l := range p.Layers() {
			element, ok := l.(*layers.Dot11InformationElement)
			// Print the full 802.11 IE fields resolved.
			s.Log(element)
			// Make sure this is MBO-OCE IE:
			// 1. At least 7 octets
			// 2. Element ID value equals 0xDD
			// 3. OUI is 0x50-6F-9A
			// 4. OUI type is 0x16
			if !ok || int(element.Length) < 7 || element.ID != 0xDD ||
				bytes.Compare(element.OUI[:3], []byte{0x50, 0x6F, 0x9A}) != 0 ||
				element.OUI[3] != 0x16 {
				continue
			}
			// Analyze the attributes in the IE.
			for i := 0; i < len(element.Info); {
				attrID := element.Info[i]
				attrLen := element.Info[i+1]
				// Check if the pattern matches the format of association disallow.
				// 1. Attribute ID is 0x04
				// 2. Attribute length is 0x01
				if attrID == 0x04 && attrLen == 0x01 {
					s.Log("MBO association disallow property successfully found in probe response")
					return nil
				}
				i += 2 + int(attrLen)
			}
		}
		return errors.New("Association disallow not found in MBO-OCE properties")
	}

	attrFound := false
	for _, p := range pac {
		layer := p.Layer(layers.LayerTypeDot11MgmtProbeResp)
		if layer == nil {
			layer = p.Layer(layers.LayerTypeDot11MgmtBeacon)
		}
		if layer == nil {
			s.Fatal("Found packet without probeResp or beacon layer")
		}
		s.Logf("The type of this packet is %s", layer.LayerType())
		// |payload| corresponds to the frame body excluding the fixed fields.
		// Generate a new packet that represents a list of IEs.
		payload := layer.LayerPayload()
		e := gopacket.NewPacket(payload, layers.LayerTypeDot11InformationElement, gopacket.NoCopy)
		var attrErr error
		if attrErr = checkAssocDisallowBit(e); attrErr == nil {
			attrFound = true
			break
		}
		s.Log("Association disallow checking failure: ", attrErr)
	}
	if !attrFound {
		s.Fatal("Association disallow configuration not found")
	}

	s.Log("Attempting to connect to AP")
	expectFailConnect := func(ctx context.Context) error {
		if _, err := tf.ConnectWifiAP(ctx, ap); err != nil {
			return nil
		}
		if err := tf.CleanDisconnectWifi(ctx); err != nil {
			s.Error("Failed to disconnect WiFi, err: ", err)
		}
		return errors.New("Unexpectedly connected to AP")
	}
	router, err := tf.StandardRouter()
	if err != nil {
		s.Fatal("Failed to get legacy router: ", err)
	}
	pcapPath, err = wifiutil.CollectPcapForAction(ctx, router, "mbo_assoc_disallow", channel, freqOpts, expectFailConnect)
	if err != nil {
		s.Fatal("Failed to collect pcap: ", err)
	}

	s.Log("Start analyzing pcap")
	filters = []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.TransmitterAddress(mac),
		pcap.TypeFilter(layers.LayerTypeDot11MgmtAssociationReq, nil),
	}
	assocPackets, err := pcap.ReadPackets(pcapPath, filters...)
	if len(assocPackets) > 0 {
		s.Fatal("DUT sent assoc requests to the AP when it shouldn't have")
	}
	s.Log("Found no association requests as expected")
}
