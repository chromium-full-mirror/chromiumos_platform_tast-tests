// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/permissionbroker"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
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
		Params: []testing.Param{
			{
				Val: false,
			},
			{
				Name: "vpn_lockdown",
				Val:  true,
			},
		},
	})
}

type routingTagSocketAPITestCase struct {
	desc          string          // description of this test case
	targetEnv     *virtualnet.Env // one peer of the socket connection
	service       *shill.Service  // another peer of the socket connection
	uid           int             // socket owner
	tagSocketOpts []patchpanel.TagSocketOption
	expectBlocked bool // whether the connection should be blocked
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
// In the VPN lockdown test, VPN server will be set up but the service property
// will be modified so that the connection cannot be established.
//
// In the tests for tagging socket for connect() (as a client):
//
// a) To verify that "call TagSocket on a socket will make the packets from this
// socket routed on a specific network", this test will set up a TCP/UDP server
// on the corresponding env only reachable from the specific network (default
// route is required to reach them), and verify that the connection can be
// established.
//
// b) To verify that "connection will be blocked in the VPN lockdown mode", this
// test will set up TCP/UDP server on the env which is supposed to be reachable
// without the VPN lockdown, and verify that the connection cannot be
// established.
//
// In the tests for tagging socket for listen() (as a server), the TCP/UDP
// server will be set up in the root netns instead, and the connection will
// initiated from the corresponding env.
//
// Subtest is used in this test since they share the same set up code, but the
// set up code is too specified to have a fixture for it.
func RoutingTagSocketAPI(ctx context.Context, s *testing.State) {
	isVPNLockdownTest := s.Param().(bool)

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Use test profile here since setting always-on VPN in the default profile
	// may be hard to recover in the worst case.
	popFunc, err := shill.LogOutUserAndPushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to push test profile: ", err)
	}
	defer popFunc(cleanupCtx)

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

	// Wait for connectivity of the high priority network to be ready. Especially
	// SLAAC may take a few seconds.
	if errs := testEnv.VerifyTestNetwork(ctx, routing.VerifyOptions{
		IPv4:      true,
		IPv6:      true,
		IsPrimary: true,
		Timeout:   30 * time.Second,
	}); len(errs) != 0 {
		s.Fatal("Failed to verify test network: ", errs)
	}

	vpnEnv := testEnv.TestRouter
	vpnOpts := []vpn.Option{vpn.WithIPType(vpn.IPTypeIPv4AndIPv6)}
	if isVPNLockdownTest {
		vpnOpts = append(vpnOpts, vpn.WithoutAutoConnect())
	}
	vpnConn, err := vpn.StartConnection(ctx, testEnv.TestRouter, vpn.TypeIKEv2, vpnOpts...)
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

	if isVPNLockdownTest {
		// Reset VPN credentials so that it cannot connect.
		if err := vpnConn.Service().ClearProperty(ctx, "IKEv2.PSK"); err != nil {
			s.Fatal("Failed to reset VPN credentials: ", err)
		}

		cleanupFunc, err := vpn.SetAlwaysOnVPN(ctx, shillconst.AlwaysOnVPNModeStrict, vpnConn.Service())
		if err != nil {
			s.Fatal("Failed to configure VPN lockdown mode: ", err)
		}
		defer cleanupFunc(cleanupCtx)

		const vpnConnectedTimeout = 2 * time.Second
		s.Logf("Wait for %v seconds to make sure VPN won't become connected", vpnConnectedTimeout.Seconds())
		// GoBigSleepLint: We want to make sure that VPN won't become connected
		// during this period, so cannot use a pool here.
		testing.Sleep(ctx, vpnConnectedTimeout)

		if connected, err := vpnConn.Service().IsConnected(ctx); err != nil {
			s.Fatal("Failed to get VPN service state: ", err)
		} else if connected {
			s.Fatal("VPN service is connected unexpected")
		}
	}

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

	// Envs for setting up one peer of the socket connection in the test cases.
	lowPrioNetworkServerEnv := testEnv.BaseServer
	highPrioNetworkServerEnv := testEnv.TestServer
	vpnReachableServerEnv := vpnPrivateEnv

	// Services (actually the corresponding interfaces) for setting up another
	// peer of the socket connection in the test cases.
	lowPrioService := testEnv.BaseService
	highPrioService := testEnv.TestService
	vpnService := vpnConn.Service()

	// Make sure that port is different in each subtest, to avoid potential with
	// port collision or connection pinning. Use a closure here to avoid exposing
	// port as a global variable.
	port := 10000
	getNextPort := func() int {
		port++
		return port
	}

	var tcs []routingTagSocketAPITestCase
	if !isVPNLockdownTest {
		vpnIfname, err := vpnConn.Service().GetDeviceInterface(ctx)
		if err != nil {
			s.Fatal("Failed to get interface name for VPN: ", err)
		}
		vpnNetworkIndex := getIfIdx(vpnIfname)

		tcs = []routingTagSocketAPITestCase{
			{
				desc:      "system traffic will be routed to low priority network with setting network_id",
				targetEnv: lowPrioNetworkServerEnv,
				service:   lowPrioService,
				uid:       rootUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
				},
			},
			{
				desc:      "user traffic will be routed to low priority network with setting network_id",
				targetEnv: lowPrioNetworkServerEnv,
				service:   lowPrioService,
				uid:       chronosUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
				},
			},
			{
				desc:      "system traffic will be routed to vpn with setting network_id",
				targetEnv: vpnReachableServerEnv,
				service:   vpnService,
				uid:       rootUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(vpnNetworkIndex),
				},
			},
			{
				desc:      "system traffic will be routed to vpn with policy=ROUTE_ON_VPN",
				targetEnv: vpnReachableServerEnv,
				service:   vpnService,
				uid:       rootUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketRouteOnVPN(),
				},
			},
			{
				desc:      "user traffic will be routed to high priority network with policy=BYPASS_VPN",
				targetEnv: highPrioNetworkServerEnv,
				service:   highPrioService,
				uid:       chronosUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketBypassVPN(),
				},
			},
		}
	} else {
		tcs = []routingTagSocketAPITestCase{
			{
				desc:      "system traffic will be routed to low priority network with setting network_id",
				targetEnv: lowPrioNetworkServerEnv,
				service:   lowPrioService,
				uid:       rootUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
				},
			},
			{
				desc:      "user traffic will be blocked with setting network_id",
				targetEnv: lowPrioNetworkServerEnv,
				service:   lowPrioService,
				uid:       chronosUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
				},
				expectBlocked: true,
			},
			{
				desc:      "system traffic will be blocked with policy=ROUTE_ON_VPN",
				targetEnv: highPrioNetworkServerEnv,
				service:   highPrioService,
				uid:       rootUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketRouteOnVPN(),
				},
				expectBlocked: true,
			},
			{
				desc:      "user traffic will be routed to high priority network with policy=BYPASS_VPN",
				targetEnv: highPrioNetworkServerEnv,
				service:   highPrioService,
				uid:       chronosUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketBypassVPN(),
				},
			},
			{
				desc:      "user traffic will be routed to low priority network with setting network_id and policy=BYPASS_VPN",
				targetEnv: lowPrioNetworkServerEnv,
				service:   lowPrioService,
				uid:       chronosUID,
				tagSocketOpts: []patchpanel.TagSocketOption{
					patchpanel.WithTagSocketNetworkID(lowPrioNetworkIndex),
					patchpanel.WithTagSocketBypassVPN(),
				},
			},
		}
	}

	for _, tc := range tcs {
		s.Run(ctx, tc.desc, func(ctx context.Context, s *testing.State) {
			if err := testConnect(ctx, tc, getNextPort); err != nil {
				s.Error("Failed to verify socket connection as client: ", err)
			}
			if err := testListen(ctx, tc, getNextPort); err != nil {
				s.Error("Failed to verify socket connection as server: ", err)
			}
		})
	}
}

// testConnect sets up a TCP/UDP server in tc.targetEnv, calls TagSocket API to
// create the socket, and verifies the connection to this server from the host.
// Connections of {TCP, UDP} x {IPv4, IPv6} will be verified.
func testConnect(ctx context.Context, tc routingTagSocketAPITestCase, getPort func() int) (retErr error) {
	ipAddrs, err := tc.targetEnv.WaitForVethInAddrs(ctx, true /*ipv4*/, true /*ipv6*/)
	if err != nil {
		return errors.Wrap(err, "failed to get IP addrs from the base server")
	}

	pp, err := patchpanel.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create patchpanel client")
	}

	switchToRootFunc, err := switchUser(ctx, tc.uid)
	if err != nil {
		return errors.Wrap(err, "failed to switch uid in setup")
	}
	defer func() {
		retErr = switchToRootFunc()
	}()

	isTCP := func(f l4server.Family) bool { return f == l4server.TCP4 || f == l4server.TCP6 }

	for _, family := range []l4server.Family{l4server.TCP4, l4server.TCP6, l4server.UDP4, l4server.UDP6} {
		port := getPort()

		server := l4server.New(family, port, l4server.WithMsgHandler(l4server.Reflector()))
		if err := tc.targetEnv.StartServer(ctx, server.String(), server); err != nil {
			return errors.Wrapf(err, "failed to start %s server", server.String())
		}

		peerSocketAddr := fmt.Sprintf("%s:%d", ipAddrs.IPv4Addr, port)
		if family == l4server.TCP6 || family == l4server.UDP6 {
			peerSocketAddr = fmt.Sprintf("[%s]:%d", ipAddrs.IPv6Addrs[0], port)
		}

		testing.ContextLogf(ctx, "Verifying %s connection to %s as client", family, peerSocketAddr)

		// Create a Dialer with modifying its behavior to call TagSocket.
		dialer := &net.Dialer{
			Control: createControlFunc(ctx, pp, tc.tagSocketOpts...),
		}

		err = triggerSocketIO(ctx, dialer, family.String(), peerSocketAddr)
		if tc.expectBlocked && err == nil {
			return errors.New("unexpected connection success")
		} else if tc.expectBlocked && err != nil {
			if isTCP(family) && strings.Contains(err.Error(), "connection refused") {
				continue
			} else if !isTCP(family) && strings.Contains(err.Error(), "operation not permitted") {
				continue
			}
			return errors.Wrap(err, "unexpected connection error")
		} else if err != nil {
			return errors.Wrap(err, "failed to trigger socket IO")
		}
	}
	return nil
}

// testListen sets up a TCP/UDP server in the root netns with calling TagSocket
// API (to simulate that the client code create a listen socket), creates a
// socket from tc.targetEnv, and verifies the connection to the server on the
// host from this socket host. Connections of {TCP, UDP} x {IPv4, IPv6} will be
// verified.
func testListen(ctx context.Context, tc routingTagSocketAPITestCase, getPort func() int) error {
	ipv4Addr, err := tc.service.GetIPv4Address(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get IPv4 address on the host side")
	}
	ipv6Addr, err := tc.service.GetIPv6Address(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get IPv6 address on the host side")
	}

	isTCP := func(f l4server.Family) bool { return f == l4server.TCP4 || f == l4server.TCP6 }

	pp, err := patchpanel.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create patchpanel client")
	}

	permBroker, err := permissionbroker.NewClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create permission broker client")
	}

	for _, family := range []l4server.Family{l4server.TCP4, l4server.TCP6, l4server.UDP4, l4server.UDP6} {
		port := getPort()

		// Request permission broker to open the port at first. Close the lifeline
		// fd to release the port.
		var err error
		var lifelineFD *os.File
		if isTCP(family) {
			lifelineFD, err = permBroker.RequestTCPPortAccess(ctx, uint16(port), "")
		} else {
			lifelineFD, err = permBroker.RequestUDPPortAccess(ctx, uint16(port), "")
		}
		if err != nil {
			return errors.Wrapf(err, "failed to open port %d", port)
		}
		defer func() {
			if err := lifelineFD.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close lifeline fd for port access")
			}
		}()

		// Start l4server in the root netns, with switching uid to tc.uid and
		// tagging the socket for the listen socket.
		server := l4server.New(family, port,
			l4server.WithMsgHandler(l4server.Reflector()),
			l4server.WithListenConfig(&net.ListenConfig{Control: createControlFunc(ctx, pp, tc.tagSocketOpts...)}),
		)
		routineSetup := func(ctx context.Context) (func() error, error) {
			return switchUser(ctx, tc.uid)
		}
		if err := server.StartWithServerRoutineSetup(ctx, routineSetup); err != nil {
			return errors.Wrap(err, "failed to start l4server locally")
		}
		defer server.Stop(ctx)

		// GoBigSleepLint: Give some time to let the l4server setup finish, and then
		// switch the user back to root, otherwise we cannot enter the other netns
		// below. This is a workaround -- unix.Setreuid() should be mapped to the
		// direct syscall on Linux, so the uid change only be applied to the current
		// thread, but it seems that the uid change is spreading to other threads
		// now. We need to figure out if this is the expected behavior or not. If
		// not, we need to use another solution in this test. Sleeping here is not
		// ideal since it may not be enough.
		testing.Sleep(ctx, 1*time.Second)
		if err := unix.Setreuid(0, 0); err != nil {
			return errors.Wrap(err, "failed to setuid back to root")
		}

		// Enter the netns of targetEnv to initiate the socket connection.
		exitNetNS, err := tc.targetEnv.EnterNetNS(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to enter netns to start client")
		}
		defer func() {
			if err := exitNetNS(); err != nil {
				testing.ContextLog(ctx, "Failed to go back to the root netns: ", err)
			}
		}()

		peerSocketAddr := fmt.Sprintf("%s:%d", ipv4Addr, port)
		if family == l4server.TCP6 || family == l4server.UDP6 {
			peerSocketAddr = fmt.Sprintf("[%s]:%d", ipv6Addr, port)
		}

		testing.ContextLogf(ctx, "Verifying %s connection to %s as server", family, peerSocketAddr)

		err = triggerSocketIO(ctx, &net.Dialer{}, family.String(), peerSocketAddr)
		if tc.expectBlocked && err == nil {
			return errors.New("unexpected connection success")
		} else if tc.expectBlocked && err != nil {
			if strings.Contains(err.Error(), "i/o timeout") {
				continue
			}
			return errors.Wrap(err, "unexpected connection error")
		} else if err != nil {
			return errors.Wrap(err, "failed to trigger socket IO")
		}
	}

	return nil
}

// createControlFunc returns a function can be used as the Control function in
// net.Dialer. This function will do the proper socket tagging with
// tagSocketOpts.
func createControlFunc(ctx context.Context, pp *patchpanel.Client, tagSocketOpts ...patchpanel.TagSocketOption) func(network, address string, conn syscall.RawConn) error {
	return func(network, address string, conn syscall.RawConn) error {
		// This function will be called after the socket is created, before
		// connect() is called.
		var tagSocketErr error
		if err := conn.Control(func(fd uintptr) {
			tagSocketErr = pp.TagSocket(ctx, int32(fd), tagSocketOpts...)
		}); err != nil {
			return err
		}
		return tagSocketErr
	}
}

// switchUser calls setreuid to switch the euid to uid, and returns a callback
// to switch the user back to root.
func switchUser(ctx context.Context, uid int) (cleanupFunc func() error, retErr error) {
	// Lock the goroutine to a thread at first since setreuid() only affects the
	// current thread.
	runtime.LockOSThread()
	defer func() {
		// Make sure to unlock if this function return error.
		if retErr != nil {
			runtime.UnlockOSThread()
		}
	}()

	// Note that we only need to change euid instead of ruid, otherwise we won't
	// be able to switch back. The following code assumes we are running as root
	// now.
	if err := unix.Setreuid(0, uid); err != nil {
		return nil, errors.Wrapf(err, "failed to setuid to %d", uid)
	}

	return func() error {
		var setUIDErr error
		if err := unix.Setreuid(0, 0); err != nil {
			setUIDErr = errors.Wrap(err, "failed to reset uid to root")
		}
		runtime.UnlockOSThread()
		return setUIDErr
	}, nil
}

// triggerSocketIO performs the following operations:
// 1. Use dialer to connect to address on network.
// 2. Send a message via the connection.
// 3. Receive a message via the connection.
// 4. Verify that the two messages are the same.
func triggerSocketIO(ctx context.Context, dialer *net.Dialer, network, address string) error {
	// Always set timeout to a smaller value since both peers are on the DUT.
	dialer.Timeout = 3 * time.Second
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return errors.Wrapf(err, "failed to dial to %s %s", network, address)
	}

	const msg = "hello"
	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return errors.Wrap(err, "failed to set write deadline on connection")
	}
	if _, err := conn.Write([]byte(msg)); err != nil {
		return errors.Wrap(err, "failed to write")
	}

	in := make([]byte, len(msg))
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return errors.Wrap(err, "failed to set read deadline on connection")
	}
	if _, err := conn.Read(in); err != nil {
		return errors.Wrap(err, "failed to read")
	}

	inStr := string(in)
	if string(in) != msg {
		return errors.Errorf("received msg does not match the sent one: got %s, expect %s", inStr, msg)
	}

	return nil
}
