// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"syscall"
	"time"

	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     RoutingTagSocketAPI,
		Desc:     "Verify the routing semantics of patchpanel TagSocket API",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

type routingTagSocketAPITestCase struct {
	desc          string          // description of this test case
	l4serverEnv   *virtualnet.Env // where the l4server should be running
	uid           int             // socket owner
	tagSocketOpts []patchpanel.TagSocketOption
}

// RoutingTagSocketAPI will set up a network topology with the virtualnet
// package as follows:
//
//	eth_test ----- router_test --+-- server_test
//	               (vpn server)  |
//	                             +-- server_vpn_private
//
//	eth_base ----- router_base --+-- server_base
//
// There will be 3 networks on DUT (ignore the real physical network): eth_test
// (high priority), eth_base (low priority), and vpn. server_vpn_private is only
// reachable from the vpn server set up in router_test.
//
// To verify that "call TagSocket on a socket will make the packets from this
// socket routed on a specific network", this test will set up a TCP/UDP server
// on the corresponding env only reachable from the specific network (default
// route is required to reach them), and verify that the connection can be
// established.
//
// Subtest is used in this test since they share the same set up code, but the
// set up code is too specified to have a fixture for it.
func RoutingTagSocketAPI(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testEnv := routing.NewTestEnv()
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	testNetworkOpts := virtualnet.EnvOptions{
		Priority:   routing.HighPriority,
		NameSuffix: routing.TestSuffix,
		EnableDHCP: true,
		RAServer:   true,
	}
	if err := testEnv.CreateNetworkEnvForTest(ctx, testNetworkOpts); err != nil {
		s.Fatal("Failed to create network for test: ", err)
	}

	vpnEnv := testEnv.TestRouter
	vpnConn, err := vpn.StartConnection(ctx, vpnEnv, vpn.TypeIKEv2, vpn.WithIPType(vpn.IPTypeIPv4AndIPv6))
	if err != nil {
		s.Fatal("Failed to setup VPN connection: ", err)
	}
	defer func() {
		if err := vpnConn.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to clean up VPN connection: ", err)
		}
	}()

	vpnPrivateEnv, err := vpn.CreatePrivateEnv(ctx, testEnv.Pool, vpnConn.Server, vpnEnv)
	if err != nil {
		s.Fatal("Failed to create env private to VPN: ", err)
	}
	defer func() {
		if err := vpnPrivateEnv.Cleanup(cleanupCtx); err != nil {
			s.Log("Failed to tear down env private to VPN: ", err)
		}
	}()

	// TODO(b/322083502): Change to use network_id when it is ready.
	getIfIdx := func(ifname string) int {
		ifi, err := net.InterfaceByName(ifname)
		if err != nil {
			s.Fatalf("Failed to get ifindex for %v: %v", ifname, err)
		}
		return ifi.Index
	}

	// The above setup is a little noisy in the log, so leave a log here to make
	// it clearer that the setup has been finished.
	s.Log("Finished setting up network topology. Starting the verification now")

	// UIDs used in the test cases.
	const chronosUID = 1000 // USER traffic
	const rootUID = 0       // SYSTEM traffic

	// Network ids used in the test cases.
	lowPrioNetworkIndex := getIfIdx(testEnv.BaseRouter.VethOutName)
	vpnIfname, err := vpnConn.Service().GetDeviceInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get interface name for VPN: ", err)
	}
	vpnNetworkIndex := getIfIdx(vpnIfname)

	// Envs for setting up the l4servers in the test cases.
	lowPrioNetworkServerEnv := testEnv.BaseServer
	highPrioNetworkServerEnv := testEnv.TestServer
	vpnReachableServerEnv := vpnPrivateEnv

	// Make sure that port is different in each subtest, to avoid potential with
	// port collision or connection pinning. Use a closure here to avoid exposing
	// port as a global variable.
	port := 10000
	getNextPort := func() int {
		port++
		return port
	}

	for _, tc := range []routingTagSocketAPITestCase{
		{
			desc:        "system traffic will be routed to low priority network with setting network_id",
			l4serverEnv: lowPrioNetworkServerEnv,
			uid:         rootUID,
			tagSocketOpts: []patchpanel.TagSocketOption{
				patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
			},
		},
		{
			desc:        "user traffic will be routed to low priority network with setting network_id",
			l4serverEnv: lowPrioNetworkServerEnv,
			uid:         chronosUID,
			tagSocketOpts: []patchpanel.TagSocketOption{
				patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
			},
		},
		{
			desc:        "system traffic will be routed to vpn with setting network_id",
			l4serverEnv: vpnReachableServerEnv,
			uid:         rootUID,
			tagSocketOpts: []patchpanel.TagSocketOption{
				patchpanel.WithTagSocketNetworkID(vpnNetworkIndex),
			},
		},
		{
			desc:        "system traffic will be routed to vpn with policy=ROUTE_ON_VPN",
			l4serverEnv: vpnReachableServerEnv,
			uid:         rootUID,
			tagSocketOpts: []patchpanel.TagSocketOption{
				patchpanel.WithTagSocketRouteOnVPN(),
			},
		},
		{
			desc:        "user traffic will be routed to high priority network with policy=BYPASS_VPN",
			l4serverEnv: highPrioNetworkServerEnv,
			uid:         chronosUID,
			tagSocketOpts: []patchpanel.TagSocketOption{
				patchpanel.WithTagSocketBypassVPN(),
			},
		},
	} {
		s.Run(ctx, tc.desc, func(ctx context.Context, s *testing.State) {
			if err := testConnect(ctx, tc, getNextPort); err != nil {
				s.Error("Failed to verify socket connection: ", err)
			}
		})
	}
}

// testConnect sets up a TCP/UDP server in the virtualnet Env, calls TagSocket
// API to create the socket, and verifies the connection to this server from the
// host. Connections of {TCP, UDP} x {IPv4, IPv6} will be verified.
func testConnect(ctx context.Context, tc routingTagSocketAPITestCase, getPort func() int) error {
	ipAddrs, err := tc.l4serverEnv.WaitForVethInAddrs(ctx, true /*ipv4*/, true /*ipv6*/)
	if err != nil {
		return errors.Wrap(err, "failed to get IP addrs from the base server")
	}

	for _, family := range []l4server.Family{l4server.TCP4, l4server.TCP6, l4server.UDP4, l4server.UDP6} {
		port := getPort()

		server := l4server.New(family, port, l4server.WithMsgHandler(l4server.Reflector()))
		if err := tc.l4serverEnv.StartServer(ctx, server.String(), server); err != nil {
			return errors.Wrapf(err, "failed to start %s server", server.String())
		}

		peerSocketAddr := fmt.Sprintf("%s:%d", ipAddrs.IPv4Addr, port)
		if family == l4server.TCP6 || family == l4server.UDP6 {
			peerSocketAddr = fmt.Sprintf("[%s]:%d", ipAddrs.IPv6Addrs[0], port)
		}

		testing.ContextLogf(ctx, "Verifying %s connection to %s", family, peerSocketAddr)

		conn, err := dialWithTagSocket(ctx, family.String(), peerSocketAddr, tc.uid, tc.tagSocketOpts...)
		if err != nil {
			return errors.Wrap(err, "failed to create socket connection")
		}

		// Send msg and read the response. Verify that they are the same.
		const msg = "hello"
		if _, err := conn.Write([]byte(msg)); err != nil {
			return errors.Wrap(err, "failed to write")
		}

		in := make([]byte, len(msg))
		if _, err = conn.Read(in); err != nil {
			return errors.Wrap(err, "failed to read")
		}

		inStr := string(in)
		if string(in) != msg {
			return errors.Errorf("received msg does not match the sent one: got %s, expect %s", inStr, msg)
		}
	}
	return nil
}

// dialWithTagSocket connects to the address on the named network. The socket
// will be created with owner uid, and before connect, it will call TagSocket on
// patchpanel to modify the socket parameters. Except for the uid and TagSocket
// parts, this function provides the same interface with net.Dial() from the
// standard library.
func dialWithTagSocket(ctx context.Context, network, address string, uid int, tagSocketOpts ...patchpanel.TagSocketOption) (_ net.Conn, retErr error) {
	pp, err := patchpanel.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create patchpanel client")
	}

	// Create a Dialer with modifying its behavior to call TagSocket.
	dialer := &net.Dialer{
		ControlContext: func(ctx context.Context, network, address string, conn syscall.RawConn) error {
			// This function will be called after the socket is created, before
			// connect() is called.
			var tagSocketErr error
			if err := conn.Control(func(fd uintptr) {
				tagSocketErr = pp.TagSocket(ctx, int32(fd), tagSocketOpts...)
			}); err != nil {
				return err
			}
			return tagSocketErr
		},
	}

	// Call setreuid() to switch to target user before creating the socket. Lock
	// the goroutine to a thread at first since setreuid() only affects the
	// current thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Note that we only need to change euid instead of ruid, otherwise we won't
	// be able to switch back. The following code assumes we are running as root
	// now.
	if err := unix.Setreuid(0, uid); err != nil {
		return nil, errors.Wrapf(err, "failed to setuid to %d", uid)
	}
	defer func() {
		// Switch back to root.
		if err := unix.Setreuid(0, 0); err != nil {
			retErr = errors.Wrap(err, "failed to reset uid to root")
		}
	}()

	return dialer.Dial(network, address)
}
