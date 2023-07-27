// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConnectMBO,
		Desc: "Verifies that the MBO IE and other MBO-related capability bits are set",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture | wificell.TFFeaturesRouterAsCapture),
		SoftwareDeps: []string{"mbo", "rrm_support"},
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				Val:               true,
			},
			{
				Name:              "marvell",
				ExtraHardwareDeps: hwdep.D(hwdep.WifiMarvell()),
				Val:               false,
			},
		},
	})
}

func ConnectMBO(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ctx, restore, err := tf.WifiClient().DisableMACRandomize(ctx)
	if err != nil {
		s.Fatal("Failed to disable MAC randomization: ", err)
	}
	defer func() {
		if err := restore(); err != nil {
			s.Error("Failed to restore MAC randomization: ", err)
		}
	}()

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
		hostapd.MBO(),
		hostapd.RRMBeaconReport(),
		hostapd.Channel(1),
		hostapd.Mode(hostapd.Mode80211acMixed),
		hostapd.HTCaps(hostapd.HTCapHT40),
		hostapd.VHTChWidth(hostapd.VHTChWidth20Or40),
	}
	pcapPath, _, err := wifiutil.ConnectAndCollectPcap(ctx, tf, apOps)
	if err != nil {
		s.Fatal("Failed to collect packet: ", err)
	}

	s.Log("Start analyzing pcap")
	filters := []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.TransmitterAddress(mac),
	}
	probePackets, err := pcap.ReadPackets(pcapPath, append(filters, pcap.TypeFilter(layers.LayerTypeDot11MgmtProbeReq, nil))...)
	if err != nil {
		s.Fatal("Failed to read probe request packets: ", err)
	}
	s.Logf("Total %d probe requests found", len(probePackets))
	assocPackets, err := pcap.ReadPackets(pcapPath, append(filters, pcap.TypeFilter(layers.LayerTypeDot11MgmtAssociationReq, nil))...)
	if err != nil {
		s.Fatal("Failed to read association request packets: ", err)
	}
	s.Logf("Total %d assoc requests found", len(assocPackets))

	checkIEs := func(p gopacket.Packet, isProbe bool) error {
		containsExt := false
		containsMBO := false
		containsRM := false
		for _, l := range p.Layers() {
			element, ok := l.(*layers.Dot11InformationElement)
			if !ok {
				continue
			}
			if element.ID == layers.Dot11InformationElementIDExtCapability {
				containsExt = true
				if int(element.Length) < 3 {
					return errors.New("Extended Capability IE not long enough")
				}
				if (element.Info[2] & 0x08) == 0 {
					return errors.New("Extended Capability IE does not contain BSS Transition capability")
				}
			}
			if element.ID == layers.Dot11InformationElementIDVendor {
				if int(element.Length) < 7 ||
					bytes.Compare(element.OUI[:3], []byte{0x50, 0x6F, 0x9A}) != 0 ||
					element.OUI[3] != 0x16 {
					continue
				}
				for i := 0; i < len(element.Info); {
					attrID := element.Info[i]
					attrLen := element.Info[i+1]
					// Check that the Cellular Data Capabilities attribute is present
					if attrID == 0x03 && attrLen == 1 && element.Info[i+2] >= 0x01 && element.Info[i+2] <= 0x03 {
						containsMBO = true
						break
					}
					i += 2 + int(attrLen)
				}
			}
			if element.ID == layers.Dot11InformationElementIDRMEnabledCapabilities {
				containsRM = true
				if int(element.Length) < 1 {
					return errors.New("RM Enabled Capabilities IE not long enough")
				}
				if (element.Info[0] & 0x10) == 0 {
					return errors.New("RM Enabled Capabilities IE missing Passive Measurement support")
				}
				if (element.Info[0] & 0x20) == 0 {
					return errors.New("RM Enabled Capabilities IE missing Active Measurement support")
				}
				if (element.Info[0] & 0x40) == 0 {
					return errors.New("RM Enabled Capabilities IE missing Table Measurement support")
				}
			}
		}
		if !containsExt {
			return errors.New("Extended Capabilities IE missing")
		} else if !containsMBO {
			return errors.New("MBO-OCE IE missing")
		} else if !isProbe && !containsRM {
			return errors.New("RM Enabled Capabilities IE missing")
		}
		return nil
	}

	// We skip the assoc request packet check for Marvell devices because
	// they are fullMAC devices, meaning wpa_supplicant can't inject the MBO
	// IEs into the assoc request packet like it does in softMAC devices.
	// We still check to make sure it can associate properly above, but it's
	// less important to check that the IEs are there.
	notMarvell := s.Param().(bool)
	if notMarvell {
		s.Log("Checking assoc request packets")
		for _, p := range assocPackets {
			if err := checkIEs(p, false); err != nil {
				s.Fatal("Assoc request IEs missing: ", err)
			}
		}
	}
}
