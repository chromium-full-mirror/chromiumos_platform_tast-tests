// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"strings"
	"time"

	pp "go.chromium.org/chromiumos/system_api/patchpanel_proto"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Threshold to make sure that we get the message packet while avoiding the
	// noise of other packets such as signaling packets (usually 50~ bytes). If
	// the byte difference is above the threshold, it means that message packet
	// is received.
	// For reference, size for message packets in this test setting are:
	// TCPv4: 453 bytes, TCPv6: 473 bytes, UDPv4: 428 bytes, UDP6: 448 bytes
	thresholdBytes = 400
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCVPNTrafficCounters,
		Desc:         "ARC VPN traffic is counted separately from ARC traffic",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		SoftwareDeps: []string{"arc", "no_android_p"},
		Params: []testing.Param{
			{
				Name: "tcp4",
				Val:  l4server.TCP4,
			}, {
				Name: "tcp6",
				Val:  l4server.TCP6,
			}, {
				Name: "udp4",
				Val:  l4server.UDP4,
			}, {
				Name: "udp6",
				Val:  l4server.UDP6,
			},
		},
	})
}

// ARCVPNTrafficCounters tests that ARC VPN traffic is counted separately from ARC traffic.
func ARCVPNTrafficCounters(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}
	// Disable captive portal check.
	restoreCaptivePortal, err := manager.DisablePortalDetectionWithRestore(ctx)
	if err != nil {
		s.Fatal("Failed to disable portal detection: ", err)
	}
	defer restoreCaptivePortal(cleanupCtx)

	restoreServiceOrder, err := manager.SetServiceOrderWithRestore(ctx, []string{"vpn", "wifi", "ethernet", "cellular"})
	if err != nil {
		s.Fatal("Failed to set service order to prioritize WiFi: ", err)
	}
	defer restoreServiceOrder(cleanupCtx)

	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	if err := a.Command(ctx, "dumpsys", "wifi", "transports", "-eth").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to disable ethernet in ARC: ", err)
	}
	defer func() {
		if err := a.Command(cleanupCtx, "dumpsys", "wifi", "transports").Run(testexec.DumpLogOnError); err != nil {
			s.Error(err, "failed to re-enable ethernet in ARC")
		}
	}()

	// Set up a WiFi router environment.
	pool := subnet.NewPool()
	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], manager, pool, virtualnet.EnvOptions{EnableDHCP: true, RAServer: true})
	if err != nil {
		s.Fatal("Failed to create WiFi router environment: ", err)
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
		s.Fatal("Failed to wait for to WiFi connected status: ", err)
	}

	routerAddrs, err := wifi.Router.WaitForVethInAddrs(ctx, true /*ipv4*/, true /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get inner addresses from router environment: ", err)
	}

	var pingAddrs []string
	pingAddrs = append(pingAddrs, routerAddrs.IPv4Addr.String())
	for _, ip := range routerAddrs.IPv6Addrs {
		pingAddrs = append(pingAddrs, ip.String())
	}

	// Ping and make sure connection has been established.
	for _, target := range pingAddrs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			s.Fatalf("network verification failed: %v is not reachable as user chronos on host: %v", target, err)
		}
	}

	// Set up L4 server based on IP family.
	networkFam := s.Param().(l4server.Family)
	var addr net.IP
	if networkFam == l4server.TCP6 || networkFam == l4server.UDP6 {
		addr = routerAddrs.IPv6Addrs[0]
	} else {
		addr = routerAddrs.IPv4Addr
	}

	// Use a random port.
	port := 65535
	server := l4server.New(networkFam, port, l4server.WithAddr(addr.String()), l4server.WithMsgHandler(l4server.Reflector()))
	if err := wifi.Router.StartServer(ctx, networkFam.String(), server); err != nil {
		s.Fatalf("Failed to start %s server: %v", networkFam, err)
	}

	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to install and preauthorizing ARC VPN test app: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	testing.ContextLog(ctx, "Starting ArcVpnTest app")
	if err := arcvpn.StartARCVPN(ctx, a); err != nil {
		s.Fatal("Failed to send ArcVpnTest app: ", err)
	}
	defer func() {
		if err := arcvpn.ForceStopARCVPN(cleanupCtx, a); err != nil {
			s.Error("Failed to clean up ARC VPN: ", err)
		}
	}()

	// Make sure the test app is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}

	ifname, err := wifi.Service.GetDeviceInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get device interface name: ", err)
	}
	// Since we cannot get enough information to choose the right network in
	// test app to setup socket, we get the ARC side interface name of
	// simulated WiFi interface in tast test and pass the information to test
	// app via intent extras.
	response, err := pc.GetDevices(ctx)
	if err != nil {
		s.Fatal("Failed to get patchpanel devices: ", err)
	}
	var guestIfname string
	for _, device := range response.Devices {
		if device.PhysIfname == ifname {
			guestIfname = device.GuestIfname
		}
	}
	if err := arcvpn.SetupSocket(ctx, a, networkFam, guestIfname, addr.String(), port); err != nil {
		s.Fatal("Failed to set socket: ", err)
	}

	// Send message packets and check difference of outgoing bytes from simulated WiFi interface.
	testing.ContextLog(ctx, "Send message packet and wait until the packet is counted")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := sendMessageAndConfirmCounted(ctx, a, networkFam, ifname, pc); err != nil {
			return errors.Wrap(err, "sent message packet is not correctly counted")
		}
		return nil
	}, &testing.PollOptions{Timeout: 40 * time.Second, Interval: 5 * time.Second}); err != nil {
		s.Fatal("Failed to wait for sent message packet to be correctly counted: ", err)
	}

	testing.ContextLog(ctx, "Check succeeded, start checking message packets sent throught the connection are all correctly counted")
	// After confirming that the message packet is correctly counted, we want
	// to check if following message packets are also correctly counted to see
	// if all the packets sent through setup connection can be correctly
	// counted.
	for i := 1; i <= 3; i++ {
		testing.ContextLogf(ctx, "Generating packets and wait for 5 seconds to collect the counter (%d/3)", i)
		if err := sendMessageAndConfirmCounted(ctx, a, networkFam, ifname, pc); err != nil {
			s.Fatal("Failed to confirm that message packet sent through this connection is correctly counted: ", err)
		}
	}
	testing.ContextLog(ctx, "Verified that the ARC VPN traffic is correctly counted")
}

// sendMessageAndConfirmCounted sends out a message and checks if the outgoing
// packet correctly counted. The traffic counters count both ARC VPN and host
// VPN traffic in one bucket, but since test environment is controlled and no
// host VPN is setup in this test, we can assume that all traffic counted in
// VPN bucket is for ARC VPN.
func sendMessageAndConfirmCounted(ctx context.Context, a *arc.ARC, fam l4server.Family, ifname string, pc *patchpanel.Client) error {
	var family pp.TrafficCounter_IpFamily
	if fam == l4server.UDP6 || fam == l4server.TCP6 {
		family = pp.TrafficCounter_IPV6
	} else {
		family = pp.TrafficCounter_IPV4
	}
	getCounters := func() (uint64, uint64, error) {
		resp, err := pc.GetTrafficCounters(ctx, []string{ifname})
		if err != nil {
			return 0, 0, errors.Wrap(err, "failed to get traffic counters")
		}
		for _, tc := range resp.Counters {
			if tc.GetIpFamily() == family && tc.GetSource() == pp.TrafficCounter_VPN {
				return tc.GetRxBytes(), tc.GetRxPackets(), nil
			}
		}
		// If no matching counters found, it means that count for the traffic counter is 0.
		return 0, 0, nil
	}

	tcByte1, tcPkt1, err := getCounters()
	if err != nil {
		return errors.Wrap(err, "failed to get traffic counters before sending message")
	}
	// Build a huge string to send a packet with larger bytes, so that the
	// packet can be distinguished from smaller signaling packets.
	var result strings.Builder
	for i := 0; i < 100; i++ {
		result.WriteString("test")
	}
	if len(result.String()) < thresholdBytes {
		return errors.New("Bytes of test string is less than threshold")
	}
	if err := arcvpn.SendMessage(ctx, a, result.String()); err != nil {
		return errors.Wrap(err, "send message failed")
	}

	// GoBigSleepLint: Ensure the counters are flushed. There is some marginal
	// delay in when the updated values are ready; and while 5 seconds is an
	// arbitrary duration, it should be the case that this much time is more
	// than enough. The alternative to sleeping here is polling and managing
	// errors. This is much simpler to maintain and understand.
	testing.Sleep(ctx, 5*time.Second)
	tcByte2, tcPkt2, err := getCounters()
	if err != nil {
		return errors.Wrap(err, "failed to get traffic counters after sending message")
	}

	// If the message packet is counted correctly, at least more than 1 packet
	// is counted and bytes difference is above threshold byte number in both iptables
	// and traffic counters.
	if (tcPkt2-tcPkt1) > 0 && (tcByte2-tcByte1) > thresholdBytes {
		return nil
	}
	if (tcPkt2 - tcPkt1) == 0 {
		return errors.New("no packet is counted")
	}
	return errors.Errorf("bytes difference is not enough: got tcByte2=%d, tcByte1=%d, expect (tcByte2-tcByte1) > %d", tcByte2, tcByte1, thresholdBytes)
}
