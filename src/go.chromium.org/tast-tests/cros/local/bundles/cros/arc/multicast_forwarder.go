// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"net"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/multicast"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// multicastForwarderTestCase defines ARC multicast lock held status
// and device power state we want to use in this test.
type multicastForwarderTestCase struct {
	// Whether ARC multicast lock is held by any app.
	multicastLockHeld bool
	// Whether device is in idle power state or not.
	deviceIdle bool
	// Whether it is a test to test WiFi multicast traffic or ethernet multicast traffic
	isWifi bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MulticastForwarder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if multicast forwarder works correctly with Android multicast lock and Android interactive state on ARC",
		Contacts:     []string{"cros-networking@google.com", "chuweih@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "wifi"},
		Timeout:      4 * time.Minute,
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		Params: []testing.Param{
			{
				Name: "idle_multicast_lock_not_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					deviceIdle:        true,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "idle_multicast_lock_not_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					deviceIdle:        true,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "idle_multicast_lock_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					deviceIdle:        true,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "idle_multicast_lock_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					deviceIdle:        true,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "interactive_multicast_lock_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					deviceIdle:        false,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "interactive_multicast_lock_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: true,
					deviceIdle:        false,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "interactive_multicast_lock_not_held_wifi",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					deviceIdle:        false,
					isWifi:            true,
				},
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name: "interactive_multicast_lock_not_held_ethernet",
				Val: multicastForwarderTestCase{
					multicastLockHeld: false,
					deviceIdle:        false,
					isWifi:            false,
				},
				ExtraSoftwareDeps: []string{"arc"},
			},
		},
	})
}

// MulticastForwarder tests that multicast traffic on WiFi is only allowed when Android
// multicast lock is held and Android power state is interactive, and multicast traffic
// on ethernet is only allowed when Android power state is interactive.
func MulticastForwarder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	isWifi := s.Param().(multicastForwarderTestCase).isWifi
	var ifnames []string

	// Hide the additional Ethernet interface (except for the one for SSH) if
	// there is any, since ARC doesn't support the 3rd interface.
	if !isWifi {
		restoreEthernet, err := arc.HideUnusedEthernet(ctx, manager)
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

	// If this is a WiFi multicast traffic test, multicast traffic should only be expected when
	// Android multicast lock is held and device is not idle. If this is an ethernet multicast
	// traffic test, multicast traffic is expected as long as device is not idle.
	multicastLockHeld := s.Param().(multicastForwarderTestCase).multicastLockHeld
	deviceIdle := s.Param().(multicastForwarderTestCase).deviceIdle
	var expectPacketReceived bool
	if isWifi {
		expectPacketReceived = multicastLockHeld && !deviceIdle
	} else {
		expectPacketReceived = !deviceIdle
	}

	if deviceIdle {
		if _, err := pc.NotifyAndroidInteractiveState(ctx, false); err != nil {
			s.Fatal("Failed to set interactive state: ", err)
		}
		defer func() {
			if _, err := pc.NotifyAndroidInteractiveState(cleanupCtx, true); err != nil {
				s.Fatal("Failed to set interactive state: ", err)
			}
		}()
	} else {
		if _, err := pc.NotifyAndroidInteractiveState(ctx, true); err != nil {
			s.Fatal("Failed to set interactive state: ", err)
		}
	}

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
		if expectPacketReceived {
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmd(ctx, tcpdumpCmd, expectOut); err != nil {
					return errors.Wrap(err, "outbound test failed")
				}
				return nil
			})
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", "arc_" + ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmd(ctx, tcpdumpCmd, expectIn); err != nil {
					return errors.Wrap(err, "inbound test failed")
				}
				return nil
			})
		} else {
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmdExpectNotFound(ctx, tcpdumpCmd, expectOut); err != nil {
					return errors.Wrap(err, "outbound test failed")
				}
				return nil
			})
			g.Go(func() error {
				tcpdumpCmd := []string{"/usr/local/sbin/tcpdump", "-Alni", "arc_" + ifname, "port", "5353", "or", "port", "1900", "-Q", "out", "--immediate-mode"}
				if err := multicast.StreamCmdExpectNotFound(ctx, tcpdumpCmd, expectIn); err != nil {
					return errors.Wrap(err, "inbound test failed")
				}
				return nil
			})
		}

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
		// Run IPv6 SSDP query
		if err := multicast.SendSSDP(ctx, multicast.SsdpUserAgentInIPv6, ifname, multicast.SsdpPort, ssdpDst); err != nil {
			s.Error("Failed starting inbound IPv6 SSDP test: ", err)
		}
	}

	if err := g.Wait(); err != nil {
		s.Fatal("Failed multicast forwarding check: ", err)
	}
}
