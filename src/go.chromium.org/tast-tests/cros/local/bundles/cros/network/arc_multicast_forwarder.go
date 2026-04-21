// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/multicast"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// multicastForwarderTestCase defines ARC multicast lock held status
// and device type in this test.
type multicastForwarderTestCase struct {
	// Whether ARC multicast lock is held by any app.
	multicastLockHeld bool
	// Whether it is a test to test WiFi multicast traffic or ethernet multicast traffic
	isWifi bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ARCMulticastForwarder,
		Desc:     "Checks if multicast forwarder works correctly with Android multicast lock held status on ARC",
		Contacts: []string{"cros-networking@google.com", "chuweih@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:network", "network_platform"},
		SoftwareDeps: []string{"chrome", "wifi"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		Params: []testing.Param{
			{
				Name: "multicast_lock_not_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			}, {
				Name: "multicast_lock_not_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			}, {
				Name: "multicast_lock_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			}, {
				Name: "multicast_lock_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
				ExtraHardwareDeps: arc.ArcAppHwDeps,
			},
		},
	})
}

// ARCMulticastForwarder tests that inbound multicast traffic on WiFi is only allowed
// when Android multicast lock is held, while outbound multicast traffic and multicast
// traffic on ethernet is always allowed.
func ARCMulticastForwarder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	isWifi := s.Param().(multicastForwarderTestCase).isWifi
	var ifnames []string

	// Hide the additional Ethernet interface (except for the one for SSH) if
	// there is any, since ARC doesn't support the 3rd interface.
	if !isWifi {
		restoreEthernet, err := arcnet.HideUnusedEthernet(ctx, manager)
		if err != nil {
			s.Fatal("Failed to hide unused ethernet: ", err)
		}
		defer restoreEthernet(cleanupCtx)
	}

	// If this test is for testing WiFi mutlicast traffic, use simulated WiFi interface.
	// If not, use ethernet interface.
	if isWifi {
		// Prepare the environment.
		simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
		pool := subnet.NewPool()
		wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], manager, pool, virtualnet.EnvOptions{EnableDHCP: true})
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
			s.Fatal("Failed to wait for to WiFi connected status: ", err)
		}

		ifnames = s.FixtValue().(*hwsim.ShillSimulatedWiFi).Client
	} else {
		pool := subnet.NewPool()
		svc, rt, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnet.EnvOptions{EnableDHCP: true})
		if err != nil {
			s.Fatal("Failed to set up ethernet network env: ", err)
		}
		defer func() {
			if rt != nil {
				rt.Cleanup(cleanupCtx)
			}
		}()

		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			s.Fatal("Failed to wait for to ethernet connected status: ", err)
		}
		ifnames = []string{rt.VethOutName}
	}

	// No valid multicast interface to test.
	if len(ifnames) == 0 {
		return
	}

	// Start ARC multicast sender app.
	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	d := s.FixtValue().(*hwsim.ShillSimulatedWiFi).UIDevice

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
	expectIn[multicast.MdnsPrefix+multicast.MdnsHostnameInIPv6] = "IPv6 mDNS"
	expectIn[multicast.MdnsPrefix+multicast.LegacyMDNSHostnameInIPv6] = "IPv6 legacy mDNS"
	// Skipped SSDP IPv6 expectations as we don't currently have the firewall rule.

	// If this is a WiFi multicast traffic test, inbound multicast traffic should only be expected
	// when Android multicast lock is held.
	// Outbound multicast traffic and multicast traffic on ethernet device is always allowed.
	multicastLockHeld := s.Param().(multicastForwarderTestCase).multicastLockHeld
	expectInboundPacketReceived := !isWifi || multicastLockHeld

	if multicastLockHeld {
		if err := d.Object(ui.ID(multicast.AcquireLockButtonID)).Click(ctx); err != nil {
			s.Error("Failed acquire lock button: ", err)
		}
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
		if expectInboundPacketReceived {
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", "arc_" + ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmd(ctx, tcpdumpCmd, expectIn); err != nil {
					return errors.Wrap(err, "inbound test failed")
				}
				return nil
			})
		} else {
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", "arc_" + ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmdExpectNotFound(ctx, tcpdumpCmd, expectIn); err != nil {
					return errors.Wrap(err, "inbound test failed")
				}
				return nil
			})
		}

	}

	s.Log("Sending IPv4 outbound multicast packets")
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
	// Run SSDP query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.SsdpUserAgentOut, multicast.SsdpButtonID, multicast.SsdpPort); err != nil {
		s.Error("Failed starting outbound SSDP test: ", err)
	}

	s.Log("Sending IPv6 outbound multicast packets")
	if err := multicast.SetIPv6Enabled(ctx, d, true); err != nil {
		s.Error("Failed to toggle IPv6: ", err)
	}
	// Send outbound multicast packets from ARC.
	// Run IPv6 mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.MdnsHostnameOutIPv6, multicast.MdnsButtonID, multicast.MdnsPort); err != nil {
		s.Error("Failed starting outbound IPv6 mDNS test: ", err)
	}
	// Run IPv6 legacy mDNS query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.LegacyMDNSHostnameOutIPv6, multicast.MdnsButtonID, multicast.LegacyMDNSPort); err != nil {
		s.Error("Failed starting outbound IPv6 legacy mDNS test: ", err)
	}
	// Run IPv6 SSDP query.
	if err := multicast.SetTextsAndClick(ctx, d, multicast.SsdpUserAgentOutIPv6, multicast.SsdpButtonID, multicast.SsdpPort); err != nil {
		s.Error("Failed starting outbound IPv6 SSDP test: ", err)
	}

	// (b/366129072) the race between outbound and inbound packets can result
	// in failure of inbound tests. Add a 5s sleep here to isolate the outbound
	// traffic and inbound traffic.
	// GoBigSleepLint: Wait for all the outbound packets to be received.
	testing.Sleep(ctx, 5*time.Second)
	s.Log("Sending IPv4 inbound multicast packets")
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
		// Run SSDP query.
		if err := multicast.SendSSDP(ctx, multicast.SsdpUserAgentIn, ifname, multicast.SsdpPort, ssdpDst); err != nil {
			s.Error("Failed starting inbound SSDP test: ", err)
		}
	}

	s.Log("Sending IPv6 inbound multicast packets")
	// Set up multicast destination addresses for IPv6 multicast.
	mdnsDst = &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353}
	ssdpDst = &net.UDPAddr{IP: net.ParseIP("ff02::c"), Port: 1900}

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
		// Run IPv6 SSDP query.
		if err := multicast.SendSSDP(ctx, multicast.SsdpUserAgentInIPv6, ifname, multicast.SsdpPort, ssdpDst); err != nil {
			s.Error("Failed starting inbound IPv6 SSDP test: ", err)
		}
	}

	if err := g.Wait(); err != nil {
		s.Fatal("Failed multicast forwarding check: ", err)
	}
}
