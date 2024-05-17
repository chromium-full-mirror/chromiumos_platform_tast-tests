// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/network/ip"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProbeReqFormat,
		Desc: "Verifies that the DUT does not send out malformed probe requests",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		// This test is not in any group because it is for manual debugging
		// a pcap problem that frame checksum might not be trust-worthy.
		// See previous investigation in b/185378075.
		Attr:        []string{"group:wificell"},
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{wificell.ShillServiceName},
		Fixture:     wificell.FixtureID(wificell.TFFeaturesCapture),
	})
}

func ProbeReqFormat(ctx context.Context, s *testing.State) {
	// Trigger active scans and verify the format of probe requests in pcap.
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

	// Collect probe requests on channel 1.
	pcapPath, err := wifiutil.ScanAndCollectPcap(ctx, tf, "malfromed_probe", 10, 1 /*channel*/, 0 /*opClass*/)
	if err != nil {
		s.Fatal("Failed to collect packet: ", err)
	}

	s.Log("Start analyzing pcap")
	filters := []pcap.Filter{
		pcap.Dot11FCSValid(),
		pcap.TransmitterAddress(mac),
		pcap.TypeFilter(layers.LayerTypeDot11MgmtProbeReq, nil),
	}
	packets, err := pcap.ReadPackets(pcapPath, filters...)
	if err != nil {
		s.Fatal("Failed to read packets: ", err)
	}
	s.Logf("Total %d probe requests found", len(packets))
	if len(packets) == 0 {
		s.Fatal("No probe request found")
	}

	for _, p := range packets {
		layer := p.Layer(layers.LayerTypeDot11MgmtProbeReq)
		if layer == nil {
			s.Fatal("Found packet without ProbeReq layer")
		}
		// Parse the frame body into IEs.
		content := layer.LayerContents()
		e := gopacket.NewPacket(content, layers.LayerTypeDot11InformationElement, gopacket.NoCopy)
		if err := e.ErrorLayer(); err != nil {
			s.Errorf("Malformed probe request %v: %v", layer, err.Error())
		}
	}
}
