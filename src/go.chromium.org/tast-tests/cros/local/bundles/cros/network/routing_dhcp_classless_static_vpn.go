// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/socketutil"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RoutingDHCPClasslessStaticVPN,
		Desc:         "Verify that VPN UID, ROUTE_ON_VPN, and BYPASS_VPN policy routing rules take precedence over DHCP Option 121 classless static route rules",
		Contacts:     []string{"cros-networking@google.com", "hugobenichi@google.com"},
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"ikev2"},
	})
}

// RoutingDHCPClasslessStaticVPN verifies that DHCPv4 Option 121 classless
// static route rules (kDstRulePriority) on a secondary physical network do not
// override VPN UID/fwmark rules (kVpnUidRulePriority) or BYPASS_VPN rules
// (kBypassVpnRulePriority) during source IP selection or packet routing.
func RoutingDHCPClasslessStaticVPN(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	testEnv := routing.NewTestEnv(nil /*cr*/)
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	testNetworkOpts := virtualnet.EnvOptions{
		Priority:   routing.LowPriority,
		NameSuffix: routing.TestSuffix,
		EnableDHCP: false,
		RAServer:   false,
	}
	if err := testEnv.CreateNetworkEnvForTest(ctx, testNetworkOpts); err != nil {
		s.Fatal("Failed to create secondary network for test: ", err)
	}

	vpnConn, err := vpn.StartConnection(ctx, testEnv.BaseRouter, vpn.TypeIKEv2)
	if err != nil {
		s.Fatal("Failed to start VPN connection on base network: ", err)
	}
	defer func() {
		if err := vpnConn.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN connection: ", err)
		}
	}()

	vpnPrivateEnv, err := vpn.CreatePrivateEnv(ctx, testEnv.Pool, vpnConn.Server, testEnv.BaseRouter)
	if err != nil {
		s.Fatal("Failed to create private env behind VPN: ", err)
	}
	defer func() {
		if err := vpnPrivateEnv.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN private env: ", err)
		}
	}()

	baseServerAddrs, err := testEnv.BaseServer.WaitForVethInAddrs(ctx, true /*ipv4*/, false /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get base server IPv4 address: ", err)
	}
	vpnServerAddrs, err := vpnPrivateEnv.WaitForVethInAddrs(ctx, true /*ipv4*/, false /*ipv6*/)
	if err != nil {
		s.Fatal("Failed to get VPN private server IPv4 address: ", err)
	}

	baseClientIP, err := testEnv.BaseService.GetIPv4Address(ctx)
	if err != nil {
		s.Fatal("Failed to get base network client IPv4 address: ", err)
	}
	vpnClientIP, err := vpnConn.Service().GetIPv4Address(ctx)
	if err != nil {
		s.Fatal("Failed to get VPN client IPv4 address: ", err)
	}

	dhcpSubnet, err := testEnv.Pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate DHCP subnet for secondary network: ", err)
	}

	// Configure the secondary network's DHCP server with Option 121 classless
	// static routes claiming both the VPN private server IP and the primary base
	// server IP. Because neither destination is actually reachable via
	// TestRouter, any traffic or source IP selection that matches kDstRulePriority
	// on the secondary network ahead of the VPN or BYPASS_VPN rules will fail.
	injectedRoutes := []dnsmasq.Route{
		{
			Prefix:  &net.IPNet{IP: vpnServerAddrs.IPv4Addr, Mask: net.IPv4Mask(255, 255, 255, 255)},
			Gateway: dhcpSubnet.GetAddrEndWith(1),
		},
		{
			Prefix:  &net.IPNet{IP: baseServerAddrs.IPv4Addr, Mask: net.IPv4Mask(255, 255, 255, 255)},
			Gateway: dhcpSubnet.GetAddrEndWith(1),
		},
	}
	dnsmasqServer := dnsmasq.New(
		dnsmasq.WithDHCPServer(dhcpSubnet),
		dnsmasq.WithDHCPClasslessStaticRoutes(injectedRoutes),
	)
	if err := testEnv.TestRouter.StartServer(ctx, "dnsmasq", dnsmasqServer); err != nil {
		s.Fatal("Failed to start dnsmasq in test router: ", err)
	}

	if err := testEnv.TestService.Reconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect secondary test service: ", err)
	}
	if err := testEnv.WaitForServiceOnline(ctx, testEnv.TestService); err != nil {
		s.Fatal("Failed to wait for secondary test service online: ", err)
	}

	// Safety check: Option 121 dst rules are in effect for untagged system traffic.
	if err := ping.ExpectPingFailure(ctx, baseServerAddrs.IPv4Addr.String(), "root"); err != nil {
		s.Fatal("Option 121 route on secondary network does not seem to be applied: ", err)
	}

	// 1. User traffic (chronos) to a destination matching Option 121 on the
	// secondary network must still select the VPN source IP and route via VPN
	// (kVpnUidRulePriority < kDstRulePriority).
	if err := ping.ExpectPingSuccessWithTimeout(ctx, vpnServerAddrs.IPv4Addr.String(), "chronos", 5*time.Second); err != nil {
		s.Fatal("Failed to ping VPN server from chronos when secondary network advertises Option 121 route: ", err)
	}
	if err := verifySocketRoute(ctx, socketutil.ChronosUID, vpnPrivateEnv, vpnServerAddrs.IPv4Addr.String(), 11001, vpnClientIP); err != nil {
		s.Fatal("Failed to verify chronos socket routing over VPN: ", err)
	}

	// 2. System traffic (root) tagged with ROUTE_ON_VPN must select the VPN
	// source IP and route via VPN (kVpnUidRulePriority < kDstRulePriority).
	if err := verifySocketRoute(ctx, socketutil.RootUID, vpnPrivateEnv, vpnServerAddrs.IPv4Addr.String(), 11002, vpnClientIP, patchpanel.WithTagSocketRouteOnVPN()); err != nil {
		s.Fatal("Failed to verify root ROUTE_ON_VPN socket routing over VPN: ", err)
	}

	// 3. User traffic (chronos) tagged with BYPASS_VPN must select the primary
	// physical network source IP and route via the primary network rather than
	// being hijacked by the secondary network's Option 121 rule
	// (kBypassVpnRulePriority < kDstRulePriority).
	if err := verifySocketRoute(ctx, socketutil.ChronosUID, testEnv.BaseServer, baseServerAddrs.IPv4Addr.String(), 11003, baseClientIP, patchpanel.WithTagSocketBypassVPN()); err != nil {
		s.Fatal("Failed to verify chronos BYPASS_VPN socket routing over primary physical network: ", err)
	}
}

// verifySocketRoute starts UDP4 and TCP4 reflector servers on port in targetEnv,
// dials them as uid with optional patchpanel socket tags, and verifies both the
// selected local source IP and end-to-end socket I/O.
func verifySocketRoute(ctx context.Context, uid int, targetEnv *env.Env, targetIP string, port int, expectedSrcIP string, tagOpts ...patchpanel.TagSocketOption) (retErr error) {
	var pp *patchpanel.Client
	if len(tagOpts) > 0 {
		var err error
		pp, err = patchpanel.New(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create patchpanel client")
		}
	}

	for _, family := range []l4server.Family{l4server.UDP4, l4server.TCP4} {
		srv := l4server.New(family, port, l4server.WithMsgHandler(l4server.Reflector()))
		if err := targetEnv.StartServer(ctx, srv.String(), srv); err != nil {
			return errors.Wrapf(err, "failed to start %s server", srv.String())
		}
	}

	switchToRootFunc, err := socketutil.SwitchUser(ctx, uid)
	if err != nil {
		return errors.Wrap(err, "failed to switch uid")
	}
	defer func() {
		if err := switchToRootFunc(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to switch uid back to root")
			}
		}
	}()

	for _, family := range []l4server.Family{l4server.UDP4, l4server.TCP4} {
		dialer := &net.Dialer{
			Timeout: 3 * time.Second,
		}
		if len(tagOpts) > 0 {
			dialer.Control = func(network, address string, rawConn syscall.RawConn) error {
				var tagErr error
				if err := rawConn.Control(func(fd uintptr) {
					tagErr = pp.TagSocket(ctx, int32(fd), tagOpts...)
				}); err != nil {
					return err
				}
				return tagErr
			}
		}

		peerAddr := fmt.Sprintf("%s:%d", targetIP, port)
		conn, err := dialer.DialContext(ctx, family.String(), peerAddr)
		if err != nil {
			return errors.Wrapf(err, "failed to dial %s %s", family, peerAddr)
		}
		defer conn.Close()

		localHost, _, err := net.SplitHostPort(conn.LocalAddr().String())
		if err != nil {
			return errors.Wrapf(err, "failed to parse local socket address %q", conn.LocalAddr().String())
		}
		if localHost != expectedSrcIP {
			return errors.Errorf("unexpected source IP for %s to %s: got %s, want %s", family, peerAddr, localHost, expectedSrcIP)
		}

		if err := socketutil.IOTest(conn); err != nil {
			return errors.Wrapf(err, "socket IO failed for %s to %s", family, peerAddr)
		}
	}
	return nil
}
