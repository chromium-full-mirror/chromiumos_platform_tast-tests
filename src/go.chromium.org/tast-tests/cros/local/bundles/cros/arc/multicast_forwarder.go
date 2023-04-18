// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"net"

	"golang.org/x/sync/errgroup"

	"chromiumos/tast/local/bundles/cros/arc/multicast"

	"go.chromium.org/tast-tests/cros/local/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MulticastForwarder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if multicast forwarder works on ARC++",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func MulticastForwarder(ctx context.Context, s *testing.State) {
	ifnames, err := multicast.SupportedInterfaces(ctx)
	if err != nil {
		s.Fatal("Failed to get multicast supported interface names: ", err)
	}
	// No valid multicast interface to test.
	if len(ifnames) == 0 {
		return
	}

	// Start ARC multicast sender app.
	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	multicast.InstallAndStartTestApp(ctx, d, a)

	// expectOut and expectIn hold expected strings to be found in the tcpdump stream
	// for outbound and inbound test respectively.
	// The value contained in the map is used for error reporting.
	expectOut := make(map[string]string)
	expectIn := make(map[string]string)

	// Adds IPv4 multicast expectations for tcpdump.
	expectOut[multicast.MdnsPrefix+multicast.MdnsHostnameOut] = "IPv4 mDNS"
	expectOut[multicast.MdnsPrefix+multicast.LegacyMDNSHostnameOut] = "IPv4 legacy mDNS"
	expectOut[multicast.SsdpPrefix+multicast.SsdpUserAgentOut] = "IPv4 SSDP"
	expectIn[multicast.MdnsPrefix+multicast.MdnsHostnameIn] = "IPv4 mDNS"
	expectIn[multicast.MdnsPrefix+multicast.LegacyMDNSHostnameIn] = "IPv4 legacy mDNS"
	expectIn[multicast.SsdpPrefix+multicast.SsdpUserAgentIn] = "IPv4 SSDP"

	// Adds IPv6 multicast expectations for tcpdump.
	expectOut[multicast.MdnsPrefix+multicast.MdnsHostnameOutIPv6] = "IPv6 mDNS"
	expectOut[multicast.MdnsPrefix+multicast.LegacyMDNSHostnameOutIPv6] = "IPv6 legacy mDNS"
	// Skipped SSDP IPv6 expectations as we don't currently have the firewall rule.
	// Skipped inbound IPv6 mDNS expectations as the lab doesn't have IPv6 connectivity.

	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		s.Fatal("Failed to check whether ARCVM is enabled: ", err)
	}

	s.Log("Starting tcpdump")
	g, ctx := errgroup.WithContext(ctx)
	for _, ifname := range ifnames {
		// Start tcpdump process.
		// In order to read the received packets directly, below flags are used:
		// * -l to make stdout line buffered,
		// * --immediate-mode to disable packet buffering.
		ifname := ifname // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
			if err := multicast.StreamCmd(ctx, tcpdumpCmd, expectOut); err != nil {
				return errors.Wrap(err, "outbound test failed")
			}
			return nil
		})
		// Skip testing inbound multicast for ARCVM.
		if vmEnabled {
			continue
		}
		g.Go(func() error {
			tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", "arc_" + ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
			if err := multicast.StreamCmd(ctx, tcpdumpCmd, expectIn); err != nil {
				return errors.Wrap(err, "inbound test failed")
			}
			return nil
		})
	}

	s.Log("Sending IPv4 multicast packets")
	if err := multicast.SetIPv6Enabled(ctx, d, false); err != nil {
		s.Error("Failed to toggle IPv6: ", err)
	}
	// Send outbound multicast packets from ARC.
	// Run mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.MdnsHostnameOut, multicast.MdnsButtonID, multicast.MdnsPort); err != nil {
		s.Error("Failed starting outbound mDNS test: ", err)
	}
	// Run legacy mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.LegacyMDNSHostnameOut, multicast.MdnsButtonID, multicast.LegacyMDNSPort); err != nil {
		s.Error("Failed starting outbound legacy mDNS test: ", err)
	}
	// Run SSDP query
	if err := multicast.SetTextsAndClick(ctx, d, multicast.SsdpUserAgentOut, multicast.SsdpButtonID, multicast.SsdpPort); err != nil {
		s.Error("Failed starting outbound SSDP test: ", err)
	}

	// Set up multicast destination addresses for IPv4 multicast.
	mdnsDst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	ssdpDst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}

	// Send inbound multicast packets by sending multicast packet that loops back.
	for _, ifname := range ifnames {
		// Run mDNS query.
		if err := multicast.SendMDNS(ctx, multicast.MdnsHostnameIn, ifname, multicast.MdnsPort, mdnsDst); err != nil {
			s.Error("Failed starting inbound mDNS test: ", err)
		}
		// Run legacy mDNS query.
		if err := multicast.SendMDNS(ctx, multicast.LegacyMDNSHostnameIn, ifname, multicast.LegacyMDNSPort, mdnsDst); err != nil {
			s.Error("Failed starting inbound legacy mDNS test: ", err)
		}
		// Run SSDP query
		if err := multicast.SendSSDP(ctx, multicast.SsdpUserAgentIn, ifname, multicast.SsdpPort, ssdpDst); err != nil {
			s.Error("Failed starting inbound SSDP test: ", err)
		}
	}

	s.Log("Sending IPv6 multicast packets")
	if err := multicast.SetIPv6Enabled(ctx, d, true); err != nil {
		s.Error("Failed to toggle IPv6: ", err)
	}
	// Send outbound multicast packets from ARC.
	// Outbound IPv6 multicast should always be tested because there is a kernel provisioned address.
	// Run IPv6 mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.MdnsHostnameOutIPv6, multicast.MdnsButtonID, multicast.MdnsPort); err != nil {
		s.Error("Failed starting outbound IPv6 mDNS test: ", err)
	}
	// Run IPv6 legacy mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.LegacyMDNSHostnameOutIPv6, multicast.MdnsButtonID, multicast.LegacyMDNSPort); err != nil {
		s.Error("Failed starting outbound IPv6 legacy mDNS test: ", err)
	}
	// Run IPv6 SSDP query
	if err := multicast.SetTextsAndClick(ctx, d, multicast.SsdpUserAgentOutIPv6, multicast.SsdpButtonID, multicast.SsdpPort); err != nil {
		s.Error("Failed starting outbound IPv6 SSDP test: ", err)
	}

	// Skip IPv6 inboud multicast test if there is no connectivity.
	if multicast.Ipv6Multicast {
		// Set up multicast destination addresses for IPv6 multicast.
		mdnsDst := &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353}
		ssdpDst := &net.UDPAddr{IP: net.ParseIP("ff02::c"), Port: 1900}

		// Send inbound multicast packets by sending multicast packet that loops back.
		for _, ifname := range ifnames {
			// Run IPv6 mDNS query.
			if err := multicast.SendMDNS(ctx, multicast.MdnsHostnameInIPv6, ifname, multicast.MdnsPort, mdnsDst); err != nil {
				s.Error("Failed starting inbound IPv6 mDNS test: ", err)
			}
			// Run IPv6 legacy mDNS query.
			if err := multicast.SendMDNS(ctx, multicast.LegacyMDNSHostnameInIPv6, ifname, multicast.LegacyMDNSPort, mdnsDst); err != nil {
				s.Error("Failed starting inbound IPv6 legacy mDNS test: ", err)
			}
			// Run IPv6 SSDP query
			if err := multicast.SendSSDP(ctx, multicast.SsdpUserAgentInIPv6, ifname, multicast.SsdpPort, ssdpDst); err != nil {
				s.Error("Failed starting inbound IPv6 SSDP test: ", err)
			}
		}
	}

	if err := g.Wait(); err != nil {
		s.Fatal("Failed multicast forwarding check: ", err)
	}
}
