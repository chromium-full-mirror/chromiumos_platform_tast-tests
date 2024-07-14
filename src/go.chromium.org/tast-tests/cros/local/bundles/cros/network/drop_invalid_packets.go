// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DropInvalidPackets,
		Desc:     "Verify that packets with IP addrs on the internal interfaces won't be sent out to physical networks",
		Contacts: []string{"cros-networking@google.com", "jiejiang@chromium.org"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		// We don't use ARC to do anything directly in this test, just to make sure
		// that packets with ARC addresses also won't be sent out.
		Fixture:      "shillSimulatedWiFiWithArcBooted",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome", "arc"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// DropInvalidPackets starts a virtual Ethernet and virtual WiFi, and verifies
// that for packets sending to these networks, if they are bound to the IP
// address from an internal interfaces (arc_* used by VMs, dns-proxy and other
// namespaces), they will be dropped. This test is for catching regressions like
// b/351809100.
func DropInvalidPackets(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewDisablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	// Start a virtual Ethernet and virtual WiFi service.
	pool := subnet.NewPool()
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	virtualnetOpts := virtualnet.EnvOptions{EnableDHCP: true}
	ethSvc, ethEnv, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnetOpts)
	if err != nil {
		s.Fatal("Failed to create virtual Ethernet env: ", err)
	}
	defer func() {
		if err := ethEnv.Cleanup(ctx); err != nil {
			s.Error("Failed to clean up virtual Ethernet env: ", err)
		}
	}()
	wifiEnv, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], manager, pool, virtualnetOpts)
	if err != nil {
		s.Fatal("Failed to create virtual WiFi env: ", err)
	}
	defer func() {
		if err := wifiEnv.Cleanup(ctx); err != nil {
			s.Error("Failed to clean up virtual WiFi env: ", err)
		}
	}()
	if err := wifiEnv.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect the WiFi service of the virtual WiFi env: ", err)
	}

	// Make sure the services are connected. Note that in this test the target
	// server is on the router so we don't care about the online state and the
	// service order.
	for _, svc := range []*shill.Service{ethSvc, wifiEnv.Service} {
		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			s.Fatalf("Failed to wait for svc %s connected: %v", svc.DBusObject, err)
		}
	}

	// Get all IPv4 addresses configured on the internal interfaces (arc_*).
	internalIPs := func() []net.IP {
		var ret []net.IP
		ifaces, err := net.Interfaces()
		if err != nil {
			s.Fatal("Failed to get network interfaces: ", err)
		}

		for _, iface := range ifaces {
			if !strings.HasPrefix(iface.Name, "arc_") {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				s.Fatalf("Failed to get addresses on %s: %v", iface.Name, err)
			}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					s.Fatalf("Failed to parse %s as CIDR: %v", addr.String(), err)
				}
				if ip.To4() != nil {
					ret = append(ret, ip)
				}
			}
		}
		return ret
	}()

	if len(internalIPs) == 0 {
		s.Fatal("No internal IPv4 addresses configured on the DUT")
	}
	s.Log("Got internal IPv4 addresses: ", internalIPs)

	// Setup finished. Start verification.
	const (
		targetPort        = 12345
		errConnRefused    = "connection refused"
		errOpNotPermitted = "operation not permitted"
	)

	// Helper function to connect to udp:remoteIP:remotePort with binding localIP
	// and expect the error contains expectErr.
	runUDPIOAndExpectErr := func(localIP, remoteIP net.IP, remotePort int, expectErr string) {
		tag := fmt.Sprintf("UDP %s -> %s:%d", localIP, remoteIP, remotePort)
		s.Log("Running `", tag, "`")
		err := func() error {
			udpConn, err := net.DialUDP("udp", &net.UDPAddr{IP: localIP, Port: 0}, &net.UDPAddr{IP: remoteIP, Port: remotePort})
			if err != nil {
				return errors.Wrap(err, "failed to connect")
			}
			bytes := []byte("hello")
			udpConn.SetDeadline(time.Now().Add(1 * time.Second))
			if _, err := udpConn.Write(bytes); err != nil {
				return errors.Wrap(err, "failed to write")
			}
			if _, err := udpConn.Read(bytes); err != nil {
				return errors.Wrap(err, "failed to read")
			}
			return nil
		}()
		if err == nil {
			s.Fatalf("`%s` succeeded, want failure %s", tag, expectErr)
		}
		if !strings.Contains(err.Error(), expectErr) {
			s.Fatalf("Unexpected failure: got %s, want %s", err.Error(), expectErr)
		}
	}
	for _, env := range []*virtualnet.Env{ethEnv, wifiEnv.Router} {
		envAddrs, err := env.GetVethInAddrs(ctx)
		if err != nil {
			s.Fatalf("Failed to get addrs inside %s: %v", env.NetNSName, err)
		}
		envIPv4Addr := envAddrs.IPv4Addr

		s.Logf("Start to verify connection behavior to %s(%s) via %s", env.NetNSName, envIPv4Addr, env.VethOutName)

		// Make sure the ping connectivity at first.
		if err := ping.ExpectPingSuccessWithTimeout(ctx, envIPv4Addr.String(), "root", 5*time.Second); err != nil {
			s.Fatalf("Failed to ping %s in %s: %v", envIPv4Addr, env.NetNSName, err)
		}

		// Baseline test: not binding to any address, expect to get an "connection
		// refused" error since there is nothing listening on that endpoint.
		// TODO(jiejiang): Ideally we should start an l4server in the corresponding
		// env and verify that the connection succeeds, but somehow doing that will
		// turn down the connection to the tast test bundle. The reason is unclear.
		runUDPIOAndExpectErr(net.IPv4zero, envIPv4Addr, targetPort, errConnRefused)

		// For all internal IPs, the packets should be dropped by iptables, so
		// expect to get an "operation not permitted" for UDP. (Note that TCP
		// handshake will time out so we use UDP here.)
		for _, internalIP := range internalIPs {
			runUDPIOAndExpectErr(internalIP, envIPv4Addr, targetPort, errOpNotPermitted)
		}
	}
}
