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
	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/qos"
	"go.chromium.org/tast-tests/cros/local/network/capture"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type packetType uint

const (
	packetTCPSyn                     packetType = 1 << iota
	packetDNS                                   = 1 << iota
	packetDHCPv4Discover                        = 1 << iota
	packetDHCPv4Request                         = 1 << iota
	packetICMPv4EchoRequest                     = 1 << iota
	packetICMPv6EchoRequest                     = 1 << iota
	packetICMPv6RouterSolicitation              = 1 << iota
	packetICMPv6NeighborSolicitation            = 1 << iota
	packetDoH                                   = 1 << iota
	packetAll                                   = (1 << iota) - 1
)

var packetTypes []packetType = []packetType{
	packetTCPSyn, packetDNS, packetDHCPv4Discover, packetDHCPv4Request,
	packetICMPv4EchoRequest, packetICMPv6EchoRequest,
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
	case packetICMPv6EchoRequest:
		return "ICMPv6 Echo Request"
	case packetICMPv6RouterSolicitation:
		return "ICMPv6 Router Solicitation"
	case packetICMPv6NeighborSolicitation:
		return "ICMPv6 Neighbor Solicitation"
	case packetAll:
		return "All"
	}
	return "unknown"
}

const (
	dscpNetworkControl uint8 = 48
	testURL                  = "http://test.example.com/"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     QosNetworkControl,
		Desc:     "Test QoS marks are correctly set on network control packets",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      2 * time.Minute,
	})
}

// QosNetworkControl verifies that DSCP marks are set on network control packets
// (DNS, DHCP, ICMP, ...) emitted by the DUT. The test uses hwsim and virtualnet
// packages to setup a wifi network and the servers required for the check. The
// packets are emitted directly from the test and DSCP mark is verified by doing
// a live capture on the access point virtual interface. The test relies on the
// test environment to guarantee that no other devices are sending network
// control packets.
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

	restoreQosFeature, err := pc.SetQosEnableWithRestore(ctx, true)
	if err != nil {
		s.Fatal("Failed to enable QoS in patchpanel: ", err)
	}
	defer restoreQosFeature(cleanupCtx)

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
	s.Logf("Starting packet capture on %s", ifaces.AP[0])
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
		RAServer:                  true,
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

	// Obtain the MAC address of the interface managed by Shill. The packets
	// send through this interface to the test gateway will have the QoS marks.
	itf, err := net.InterfaceByName(ifaces.Client[0])
	if err != nil {
		s.Fatalf("Failed to obtain %s interface", ifaces.Client[0])
	}
	clientMac := itf.HardwareAddr

	conf, err := wifi.Service.GetCurrentIPConfig(ctx)
	if err != nil {
		s.Fatal("Failed to obtain service WiFi conf: ", err)
	}

	props, err := conf.GetIPProperties(ctx)
	if err != nil {
		s.Fatal("Failed to obtain service IP properties: ", err)
	}
	s.Logf("IP configuration: client=%s gateway=%s", props.Address, props.Gateway)

	s.Logf("Generating HTTP GET request on %s", testURL)

	// Do a GET request to the webserver running on the gateway to generate DNS
	// and TCP packets.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		r, err := http.Get(testURL)
		if err != nil {
			return err
		}
		if r.StatusCode != http.StatusNoContent {
			return errors.Errorf("unexpected status code %d", r.StatusCode)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: time.Second,
	}); err != nil {
		s.Fatalf("Failed to GET %s: %v", testURL, err)
	}

	s.Log("Generating ping to the gateway")

	// Ping all the addresses of the gateway address to generate ICMP echo requests.
	gwAddrs, err := wifi.Router.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to obtain gateway addresses: ", err)
	}
	for _, addr := range gwAddrs.All() {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, addr.String(), "root", 10*time.Second); err != nil {
			s.Fatalf("Failed to ping %s: %v", addr, err)
		}
	}

	s.Log("Generating DNS over HTTPS requests to the gateway")

	// Install test certificates for HTTPS server. In doing so, virtualnet/certs will mount a test certificate directory.
	// Because DNS proxy lives in its own namespace, it needs to be restarted to be able to see the test certificates.
	httpsCerts := certs.New(certs.SSLCrtPath, certificate.TestCert3())
	cleanupCerts, err := httpsCerts.InstallTestCerts(ctx)
	if err != nil {
		s.Fatal("Failed to setup certificates: ", err)
	}
	defer cleanupCerts(cleanupCtx)
	if err := dns.RestartDNSProxy(ctx); err != nil {
		s.Fatal("Failed to restart DNS proxy: ", err)
	}

	dohServer := httpserver.New(httpserver.TCP4, "443", dns.DoHResponder(ctx, props.Gateway), httpsCerts)
	if err := wifi.Router.StartServer(ctx, "doh_server", dohServer); err != nil {
		s.Fatal("Failed to start DoH HTTPS server: ", err)
	}

	if err := m.SetDNSProxyDOHProviders(ctx, map[string]interface{}{dns.ExampleDoHProvider: ""}); err != nil {
		s.Fatal("Failed to set dns-proxy DoH providers: ", err)
	}
	defer func() {
		if err := m.SetDNSProxyDOHProviders(cleanupCtx, map[string]interface{}{}); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to set reset dns-proxy DoH providers: ", err)
		}
	}()

	// DoH providers are propagated to DNS proxy from shill call above.
	// The propagation is expected to happen very quickly (<100 ms).
	// In order to avoid flakiness on edge cases, add sleep. The test is
	// expected to pass even without the sleep on the normal case.
	// GoBigSleepLint: The sleep is necessary as there is currently no
	// way of querying DNS proxy's state.
	// TODO(b/281778714): Add a way to query DNS proxy's state.
	testing.Sleep(ctx, 1*time.Second)

	// Generate requests from different users to let dnsproxy do DoH requests.
	// TODO(b/296958870): add a test for Chrome that has its own DoH flow.
	tc := []dns.ProxyTestCase{
		{Client: dns.System, AllowRetry: true},
		{Client: dns.User, AllowRetry: true},
	}
	if errs := dns.TestQueryDNSProxy(ctx, tc, nil /* arc */, nil /* container */, dns.NewQueryOptions()); len(errs) != 0 {
		for _, err := range errs {
			s.Fatal("Failed DNS query check: ", err)
		}
	}

	// Create a shortened context to bound the packet checks. As the connection
	// is already done, it shouldn't be necessary to wait a long time.
	d := time.Now().Add(10 * time.Second)
	shortCtx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	if err := checkPacketsMarks(shortCtx, capturer.Packets(), clientMac); err != nil {
		s.Fatal("Failed to check packets marks: ", err)
	}
}

func checkPacketsMarks(ctx context.Context, packets chan *capture.Packet, clientMac net.HardwareAddr) error {
	// Check for the packets received
	seen := packetType(0)
	for seen != packetAll {
		select {
		case p := <-packets:
			// Check DHCP v4 packet marks. The packets are emitted by the DUT
			// after layer 2 is connected. As we're only interested by the
			// Discover and Request packet, we can filter them ensuring the
			// destination address is a broadcast one.
			if p.DHCPv4 != nil && p.IPv4.DstIP.Equal(net.IPv4bcast) {
				// All DHCP emitted by the DUT are expected to be marked with DSCP 48.
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("DHCPv4 packet marked with wrong DSCP: %s", p)
				}
				switch getDHCPMsgType(p) {
				case layers.DHCPMsgTypeDiscover:
					seen |= packetDHCPv4Discover
				case layers.DHCPMsgTypeRequest:
					seen |= packetDHCPv4Request
				}
				continue
			}

			if p.ICMPv4 != nil && p.ICMPv4.TypeCode.Type() == layers.ICMPv4TypeEchoRequest {
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("ICMPv4 packet marked with wrong DSCP: %s", p)
				}
				seen |= packetICMPv4EchoRequest
				continue
			}

			if isFromMAC(p, clientMac) && p.ICMPv6 != nil && p.ICMPv6.TypeCode.Type() == layers.ICMPv6TypeEchoRequest {
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("ICMPv6 Echo Request marked with wrong DSCP: %s", p)
				}
				seen |= packetICMPv6EchoRequest
				continue
			}

			if isFromMAC(p, clientMac) && p.ICMPv6 != nil && p.ICMPv6.TypeCode.Type() == layers.ICMPv6TypeRouterSolicitation {
				// b/314266135: for now we can't strictly check that RS packets
				// are marked since ndproxy uses raw packet socket that bypasses
				// iptables.
				seen |= packetICMPv6RouterSolicitation
				continue
			}

			if isFromMAC(p, clientMac) && p.ICMPv6 != nil && p.ICMPv6.TypeCode.Type() == layers.ICMPv6TypeNeighborSolicitation {
				// b/314266135: for now we can't strictly check that NS packets
				// are marked since ndproxy uses raw packet socket that bypasses
				// iptables.
				seen |= packetICMPv6NeighborSolicitation
				continue
			}

			// Check outgoing DNS packet mark. Outgoing packets are filtered
			// using the gateway address based on the assumption that in the
			// test environment, network control packets are only sent to the
			// gateway.
			if isFromMAC(p, clientMac) && p.DNS != nil {
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("DNS packet marked with wrong DSCP: %s", p)
				}
				seen |= packetDNS
				continue
			}

			if isFromMAC(p, clientMac) && p.TCP != nil && p.TCP.DstPort == 443 {
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("DoH/HTTPS/TCP packet marked with wrong DSCP: %s", p)
				}
				seen |= packetDoH
				continue
			}

			// Check outgoing TCP SYN packet mark.
			if isFromMAC(p, clientMac) && p.TCP != nil && p.TCP.SYN {
				if !qos.HasDSCP(p, dscpNetworkControl) {
					return errors.Errorf("TCP SYN packet marked with wrong DSCP: %s", p)
				}
				seen |= packetTCPSyn
				continue
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

// isFromMAC returns true when the packets comes from the client interface.
func isFromMAC(p *capture.Packet, src net.HardwareAddr) bool {
	return p.Ethernet != nil && p.Ethernet.SrcMAC.String() == src.String()
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
