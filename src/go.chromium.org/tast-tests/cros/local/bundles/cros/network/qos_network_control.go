// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/gopacket/layers"
	"go.chromium.org/tast-tests/cros/local/network/capture"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type packetType uint

const (
	packetTCPSyn            packetType = 1 << iota
	packetDNS                          = 1 << iota
	packetDHCPv4Discover               = 1 << iota
	packetDHCPv4Request                = 1 << iota
	packetICMPv4EchoRequest            = 1 << iota
	packetAll                          = (1 << iota) - 1
)

var packetTypes []packetType = []packetType{
	packetTCPSyn, packetDNS, packetDHCPv4Discover, packetDHCPv4Request, packetICMPv4EchoRequest,
}

func (pt packetType) String() string {
	switch pt {
	case packetTCPSyn:
		return "TCP SYN"
	case packetDNS:
		return "DNS"
	case packetDHCPv4Discover:
		return "DHCPv4 Discover"
	case packetDHCPv4Request:
		return "DHCPv4 Request"
	case packetICMPv4EchoRequest:
		return "ICMPv4 Echo Request"
	case packetAll:
		return "All"
	}
	return "unknown"
}

const (
	dscpNetworkControl uint8 = 48
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     QosNetworkControl,
		Desc:     "Test QoS marks are correctly set on network control packets",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "shill-wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      2 * time.Minute,
	})
}

// QosNetworkControl verifies that DSCP marks are set on network control packets
// (DNS, DHCP, ICMP, ...) emitted by the DUT. The test uses hwsim and virtualnet
// packages to setup a wifi network and the servers required for the check. The
// packets are emitted directly from the test and DSCP mark is verified by doing
// a live capture on the access point virtual interface. It typically verifies
// that at least one packet of a given protocol has the correct mark.
// TODO(b/296958870): check ICMPv6 packet marks.
func QosNetworkControl(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Obtain the simulated interfaces from the fixture environment.
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)

	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}

	if err := pc.SetQosEnable(ctx, true); err != nil {
		s.Fatal("Failed to enable QoS in patchpanel: ", err)
	}
	defer func() {
		if err := pc.SetQosEnable(cleanupCtx, false); err != nil {
			s.Error("Failed to disable QoS in patchpanel: ", err)
		}
	}()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	// Enable DHCP QoS.
	restoreDHCPQos, err := m.SetEnableDHCPQosWithRestore(ctx, true)
	if err != nil {
		s.Fatal("Failed to enable QoS for DHCP in Shill: ", err)
	}
	defer restoreDHCPQos(cleanupCtx)

	// Disable captive portal check.
	restoreCaptivePortal, err := m.DisablePortalDetectionWithRestore(ctx)
	if err != nil {
		s.Fatal("Failed to disable portal detection: ", err)
	}
	defer restoreCaptivePortal(cleanupCtx)

	// Re-order services to put WiFi on top of ethernet and let the test access
	// the WiFi network.
	restoreServiceOrder, err := m.SetServiceOrderWithRestore(ctx, []string{"vpn", "wifi", "ethernet", "cellular"})
	if err != nil {
		s.Fatal("Failed to set WiFi service order: ", err)
	}
	defer restoreServiceOrder(cleanupCtx)

	// Start packet capture.
	capturer := capture.NewCapturer(ifaces.AP[0])
	if err := capturer.Start(ctx); err != nil {
		s.Fatal("Failed to start capture: ", err)
	}
	defer capturer.Stop()

	// Create the virtual environment.
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, ifaces.AP[0], m, pool, virtualnet.EnvOptions{
		EnableDHCP:                true,
		EnableDNS:                 true,
		HTTPServerResponseHandler: httpserver.NoContentHandler,
	})
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}

	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for WiFi connected status: ", err)
	}

	conf, err := wifi.Service.GetCurrentIPConfig(ctx)
	if err != nil {
		s.Fatal("Failed to obtain service WiFi conf: ", err)
	}

	props, err := conf.GetIPProperties(ctx)
	if err != nil {
		s.Fatal("Failed to obtain service IP properties: ", err)
	}

	s.Logf("IP configuration: client=%s gateway=%s", props.Address, props.Gateway)

	r, err := http.Get("http://test.example.com/")
	if err != nil {
		s.Fatal("Failed to perform GET request: ", err)
	}
	if r.StatusCode != http.StatusNoContent {
		s.Fatalf("Unexpected GET status code %d", r.StatusCode)
	}

	if err := ping.ExpectPingSuccessWithTimeout(ctx, props.Gateway, "root", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s: %v", props.Gateway, err)
	}

	// Create a shortened context to bound the packet checks. As the connection
	// is already done, it shouldn't be necessary to wait a long time.
	d := time.Now().Add(10 * time.Second)
	shortCtx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	if err := checkPacketsMarks(shortCtx, capturer.Packets(), props.Address); err != nil {
		s.Fatal("Failed to check packets marks: ", err)
	}
}

func checkPacketsMarks(ctx context.Context, packets chan *capture.Packet, clientIP string) error {
	clientAddr := net.ParseIP(clientIP)

	// Check for the packets received
	seen := packetType(0)
	for seen != packetAll {
		select {
		case p := <-packets:
			// Check DHCP v4 packet marks. The packets are emitted by the DUT
			// after layer 2 is connected.
			if p.DHCPv4 != nil && hasDSCP(p, dscpNetworkControl) {
				switch getDHCPMsgType(p) {
				case layers.DHCPMsgTypeDiscover:
					seen |= packetDHCPv4Discover
				case layers.DHCPMsgTypeRequest:
					seen |= packetDHCPv4Request
				}
				continue
			}

			if p.IPv4 != nil && !p.IPv4.SrcIP.Equal(clientAddr) || p.IPv6 != nil {
				// For now ignore IPv6 packets or packets not explicitly emitted by the client.
				continue
			}

			// Check DNS packet mark.
			if p.DNS != nil && hasDSCP(p, dscpNetworkControl) {
				seen |= packetDNS
				continue
			}

			// Check TCP SYN packet mark.
			if p.TCP != nil && p.TCP.SYN && hasDSCP(p, dscpNetworkControl) {
				seen |= packetTCPSyn
				continue
			}

			if p.ICMPv4 != nil && p.ICMPv4.TypeCode.Type() == layers.ICMPv4TypeEchoRequest && hasDSCP(p, dscpNetworkControl) {
				seen |= packetICMPv4EchoRequest
			}

		case <-ctx.Done():
			return errors.Errorf("timeout waiting for packets: %s", listPackets(seen^packetAll))
		}

		if seen == packetAll {
			break
		}
	}
	return nil
}

func hasDSCP(p *capture.Packet, dscp uint8) bool {
	return p.DSCP() == dscp
}

func getDHCPMsgType(p *capture.Packet) layers.DHCPMsgType {
	if p.DHCPv4 != nil {
		for _, op := range p.DHCPv4.Options {
			if op.Type != layers.DHCPOptMessageType {
				continue
			}
			return layers.DHCPMsgType(op.Data[0])
		}
	}
	return layers.DHCPMsgTypeUnspecified
}

func listPackets(pktBits packetType) string {
	var packets []string
	for _, t := range packetTypes {
		if pktBits&t == t {
			packets = append(packets, t.String())
		}
	}
	return strings.Join(packets, ",")
}
