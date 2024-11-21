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

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/socketutil"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     VPNDestroySockets,
		Desc:     "Verify the socket destruction logic in shill after VPN is disconnected",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{
			{
				Val: false,
			},
			{
				Name: "split_routing",
				Val:  true,
			},
		},
	})
}

func VPNDestroySockets(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewResetVirtualnetHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	splitRouting := s.Param().(bool)

	// Set up VPN. The test will trigger the connect manually later so do not auto
	// connect when creating the VPN connection.
	opts := []vpn.Option{
		vpn.WithoutAutoConnect(),
	}
	if splitRouting {
		// Add 10.0.0.0/8 as included route, which covers the default overlay subnet
		// for the VPN service created by the vpn package. 8 is the threshold used
		// in shill to determine if a VPN service is split-routing or not. See
		// platform2/shill/vpn/vpn_util.h:InferIsUsedAsDefaultGatewayFromIncludedRoutes()
		// for details.
		opts = append(opts, vpn.WithIPv4IncludedRoute(&net.IPNet{
			IP:   net.ParseIP("10.0.0.0"),
			Mask: net.CIDRMask(8, 32),
		}))
	}

	vpnConn, err := vpn.StartConnection(ctx, nil /*env*/, vpn.TypeIKEv2, opts...)
	if err != nil {
		s.Fatal("Failed to create VPN connection: ", err)
	}

	// Set up socket connections.
	env := vpnConn.Server.Env()
	addrs, err := env.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get addrs in VPN env: ", err)
	}

	nextPort := 10000
	createSockConnForUID := func(uid int, family l4server.Family) net.Conn {
		port := nextPort
		nextPort++

		server := l4server.New(family, port, l4server.WithMsgHandler(l4server.Reflector()))
		if err := env.StartServer(ctx, server.String(), server); err != nil {
			s.Fatalf("Failed to start %s server: %v", server.String(), err)
		}

		var addr string
		switch family {
		case l4server.TCP4, l4server.UDP4:
			addr = fmt.Sprintf("%s:%d", addrs.IPv4Addr, port)
		case l4server.TCP6, l4server.UDP6:
			addr = fmt.Sprintf("[%s]:%d", addrs.IPv6Addrs[0], port)
		}

		backToRoot, err := socketutil.SwitchUser(ctx, uid)
		if err != nil {
			s.Fatalf("Failed to switch user to %v: %v", uid, err)
		}
		defer func() {
			if err := backToRoot(); err != nil {
				s.Fatal("Failed to switch back to root: ", err)
			}
		}()

		conn, err := net.Dial(family.String(), addr)
		if err != nil {
			s.Fatalf("Failed to connect to %s: %v", server.String(), err)
		}
		if err := socketutil.IOTest(conn); err != nil {
			s.Fatalf("Failed to do IO test with %s: %v", server.String(), err)
		}
		return conn
	}

	var rootSockConns []net.Conn
	var shillSockConns []net.Conn
	var chronosSockConns []net.Conn
	for _, family := range []l4server.Family{l4server.TCP4, l4server.TCP6, l4server.UDP4, l4server.UDP6} {
		rootSockConns = append(rootSockConns, createSockConnForUID(socketutil.RootUID, family))
		shillSockConns = append(rootSockConns, createSockConnForUID(socketutil.ShillUID, family))
		chronosSockConns = append(chronosSockConns, createSockConnForUID(socketutil.ChronosUID, family))
	}

	// Connect the VPN service. Socket should be destroyed if the VPN is not
	// split-routing.
	if err := vpnConn.Connect(ctx); err != nil {
		s.Fatal("Failed to connect the VPN service: ", err)
	}

	// Expected err msg after the socket is destroyed.
	const (
		expectedTCPErr = "software caused connection abort"
		expectedUDPErr = "destination address required"
	)

	// Sockets owned by root or shill should not be destroyed.
	for _, conn := range rootSockConns {
		if err := socketutil.IOTest(conn); err != nil {
			s.Fatal("Failed to do socket IO for root socket: ", err)
		}
	}
	for _, conn := range shillSockConns {
		if err := socketutil.IOTest(conn); err != nil {
			s.Fatal("Failed to do socket IO for shill socket: ", err)
		}
	}

	// Sockets owned by chronos should only be destroyed if the VPN is not
	// split-routing.
	for _, conn := range chronosSockConns {
		err := socketutil.IOTest(conn)
		if splitRouting {
			if err != nil {
				s.Fatal("Failed to do socket IO for chronos socket: ", err)
			}
		} else {
			if err == nil {
				s.Fatal("Unexpected socket IO success for chronos socket")
			} else if !strings.Contains(err.Error(), expectedTCPErr) && !strings.Contains(err.Error(), expectedUDPErr) {
				s.Fatal("Unexpected socket IO failure for chronos socket: ", err)
			}
		}
	}
}
