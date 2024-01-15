// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/webrtc"
	"go.chromium.org/tast-tests/cros/local/network/capture"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     QosWebRTC,
		Desc:     "Test QoS marks are correctly set on WebRTC packets",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		// TODO(b/317282580#comment2): Remove `no_kernel_upstream` once
		// crrev.com/c/5119091 is in the kernel upstream branch.
		SoftwareDeps: []string{"bpf", "wifi", "chrome", "no_kernel_upstream"},
		Data:         webrtc.TestFiles(),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      5 * time.Minute, // need to start Chrome in the test
	})
}

// QosWebRTC sets up a WebRTC environment where two RTC peers can be talked with
// each other directly by NAT (but not via a TURN server), and verifies that if
// there are enough packets marked with the specific DSCP value passing through
// the virtual WiFi interface.
//
// The following network topology is set up in this test:
//
//	         +---------------------+
//	         |  virtualnet.Env     |
//	         |                     |
//	wlan  <--+--> wlan_in  <-+     |
//	         |               |     |
//	         |              NAT    |
//	         |               |     |
//	veth  <--+--> veth_in  <-+     |
//	         |                     |
//	         +---------------------+
//
// The two RPC peers (naming them as "local" and "remote" for the ease of
// discussion) use the wlan and veth correspondingly. Packets will be sent out
// from wlan, get forwarded in the netns, and finally reach the remote peer at
// veth. In the test we care about the packets sending out on wlan. To make sure
// the packets traverse the netns, both peers cannot know the physical address
// of each other directly (otherwise the packets will go through the loopback
// interface since the address is on the host). Instead, we inject fake
// IceCandidates to let the local peer think that the remote is on wlan_in and
// let the remote peer think that the local is on veth_in, and also sets up the
// iptables NAT rules in netns to make them able to talk to each other.
func QosWebRTC(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Set up environment for WebRTC.
	webrtcEnv := webrtc.NewEnv()
	defer webrtcEnv.TearDown(cleanupCtx)
	if err := webrtcEnv.SetUp(ctx, s.DataFileSystem()); err != nil {
		s.Fatal("Failed to set up WebRTC test environment: ", err)
	}

	// Enable QoS feature.
	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}
	restoreQosFeature, err := pc.SetQosEnableWithRestore(ctx, true)
	if err != nil {
		s.Fatal("Failed to enable QoS in patchpanel: ", err)
	}
	defer restoreQosFeature(cleanupCtx)

	// Create the virtual WiFi environment.
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	apInterface := s.FixtValue().(*hwsim.ShillSimulatedWiFi).AP[0]
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, apInterface, m, pool, virtualnet.EnvOptions{
		EnableDHCP: true,
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
		s.Fatal("Failed to obtain IPConfig on the WiFi service: ", err)
	}

	props, err := conf.GetIPProperties(ctx)
	if err != nil {
		s.Fatal("Failed to obtain service IP properties: ", err)
	}
	s.Logf("IP configuration: client=%s gateway=%s", props.Address, props.Gateway)

	rtcLocalIP := props.Address
	rtcLocalIPInNetns := props.Gateway

	// Set another veth pair to connect the root netns and wifi netns, and
	// configure the routing setup properly. Shill won't manage the created
	// interface, since 1) it will be tedious to configure static IP via shill; 2)
	// we use on-link addresses on this interface, which does not rely on the
	// routing setup in shill.
	//
	// The IP address on the interface created inside the netns will be returned.
	// We need it in the following WebRTC setup.
	rtcRemoteIPInNetns := func() string {
		ns := wifi.Router.NetNSName

		// With prefix "veth" so that it won't be managed by shill.
		rtcRemoteIfname := "vetho_remote"
		rtcRemoteIfnameInNetns := "vethi_remote"

		// Create the veth pair. We don't need to clean them up explicitly, since
		// they will automatically removed when the wifi netns is removed.
		if err := testexec.CommandContext(ctx,
			"ip", "link", "add", rtcRemoteIfname, "type", "veth",
			"peer", rtcRemoteIfnameInNetns, "netns", ns).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to create the veth pair for WebRTC peer")
		}

		// Bring both peers up.
		if err := testexec.CommandContext(ctx, "ip", "link", "set", rtcRemoteIfname, "up").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to bring veth up")
		}
		if err := wifi.Router.RunWithoutChroot(ctx, "ip", "link", "set", rtcRemoteIfnameInNetns, "up"); err != nil {
			s.Fatal("Failed to bring veth up in netns")
		}

		// Allocate static IPs.
		v4Subnet, err := pool.AllocNextIPv4Subnet()
		if err != nil {
			s.Fatal("Failed to create IPv4 subnet for the WebRTC peer: ", err)
		}
		rtcRemoteIP := v4Subnet.GetAddrEndWith(2).String()
		rtcRemoteIPInNetns := v4Subnet.GetAddrEndWith(1).String()

		if err := testexec.CommandContext(ctx, "ip", "addr", "add", rtcRemoteIP+"/24", "dev", rtcRemoteIfname).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to add ip address to interface: ", err)
		}
		if err := wifi.Router.RunWithoutChroot(ctx, "ip", "addr", "add", rtcRemoteIPInNetns+"/24", "dev", rtcRemoteIfnameInNetns); err != nil {
			s.Fatal("Failed to add ip address to interface in the netns: ", err)
		}

		rtcLocalIfnameInNetns := wifi.Router.VethInName

		// Set up SNAT and DNAT so that:
		// - Packets from iface=rtcLocalIfnameInNetns with (src=rtcLocalIP, dst=rtcLocalIPInNetns)
		//   will go via  iface=rtcRemoteIfnameInNetns with (src=rtcRemoteIPInNetns, dst=rtcRemoteIP)
		// - Packets from iface=rtcRemoteIfnameInNetns with (src=rtcRemoteIP, dst=rtcRemoteIPInNetns)
		//   will go via  iface=rtcLocalIfnameInNetns with (src=rtcLocalIPInNetns, dst=rtcLocalIP)
		for _, c := range []struct {
			ifname string
			addr   string
		}{{rtcLocalIfnameInNetns, rtcRemoteIP},
			{rtcRemoteIfnameInNetns, rtcLocalIP}} {
			if err := wifi.Router.RunWithoutChroot(ctx,
				"iptables", "-t", "nat", "-A", "PREROUTING",
				"-i", c.ifname,
				"-j", "DNAT", "--to-destination", c.addr, "-w"); err != nil {
				s.Fatal("Failed to install iptables rules: ", err)
			}
		}
		for _, c := range []struct {
			ifname string
			addr   string
		}{{rtcLocalIfnameInNetns, rtcLocalIPInNetns},
			{rtcRemoteIfnameInNetns, rtcRemoteIPInNetns}} {
			if err := wifi.Router.RunWithoutChroot(ctx,
				"iptables", "-t", "nat", "-A", "POSTROUTING",
				"-o", c.ifname,
				"-j", "SNAT", "--to-source", c.addr, "-w"); err != nil {
				s.Fatal("Failed to install iptables rules: ", err)
			}
		}

		return rtcRemoteIPInNetns
	}()

	// Open the WebRTC webpage.
	if err := webrtcEnv.StartConn(ctx, s.OutDir(), rtcRemoteIPInNetns, rtcLocalIPInNetns); err != nil {
		s.Fatal("Failed to open the WebRTC webpage: ", err)
	}

	// Start packet capture and check that if there are expectedPktCnt WebRTC
	// packets (matched by protocol and IPs) configured dscpMultimediaConferencing
	// in the DSCP fields.
	const (
		maxDuration                = 10 * time.Second
		expectedPktCnt             = 100
		dscpMultimediaConferencing = 34
	)

	s.Logf("Starting packet capture on %s", apInterface)
	capturer := capture.NewCapturer(apInterface)
	if err := capturer.Start(ctx); err != nil {
		s.Fatal("Failed to start capture: ", err)
	}
	defer capturer.Stop()

	d := time.Now().Add(maxDuration)
	shortCtx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	webrtcPktCnt := 0
	for webrtcPktCnt < expectedPktCnt {
		select {
		case p := <-capturer.Packets():
			if p.UDP == nil {
				continue
			}
			if p.IPv4 != nil && p.IPv4.SrcIP.String() == rtcLocalIP && p.IPv4.DstIP.String() == rtcLocalIPInNetns {
				if p.DSCP() == dscpMultimediaConferencing {
					webrtcPktCnt++
				}
			}
		case <-shortCtx.Done():
			s.Fatalf("Failed to get enough number of packets with DSCP mark: got %v, want %v", webrtcPktCnt, expectedPktCnt)
		}
	}
}
