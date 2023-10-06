// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	rep "chromiumos/reporting"
	xdr "chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

type networkProtocolDetails struct {
	senderCmd   *testexec.Cmd
	receiverCmd *testexec.Cmd
	protocol    string
	ipAddr      string
}

type networkType string

type networkTypeParams struct {
	protocol networkType
	family   l4server.Family
}

type server struct {
	port int
	addr net.IP
}

const (
	icmp  networkType = "ICMP"
	tcp   networkType = "TCP"
	tcpV6 networkType = "TCPV6"
	udp   networkType = "UDP"
	udpV6 networkType = "UDPV6"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NetworkEvents,
		Desc: "Checks that XDR network events are correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
			"yanghenry@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			// TODO: According to jiejiang@, icmp can be tested by simply sending pings,
			// hence it might not not necessary to separate as an individual test.
			Name: "icmp",
			Val: networkTypeParams{
				protocol: icmp,
				family:   l4server.TCP4,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp",
			Val: networkTypeParams{
				protocol: tcp,
				family:   l4server.TCP4,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp_v6",
			Val: networkTypeParams{
				protocol: tcpV6,
				family:   l4server.TCP6,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp",
			Val: networkTypeParams{
				protocol: udp,
				family:   l4server.UDP4,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp_v6",
			Val: networkTypeParams{
				protocol: udpV6,
				family:   l4server.UDP6,
			},
		}},
	})
}

// NetworkEvents trigger network events,
// and verifies it against the events emitted by secagentd over dbus.
// Note that the receiver doesn't need to be reachable, it's more testing the outgoing packets.
func NetworkEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()

	// On test failure save off the kernel trace.
	defer func() {
		if err := secagentdcommon.OnErrorSaveKernelTrace(cleanupCtx, s.OutDir(), s.HasError); err != nil {
			s.Logf("Unable to export kernel traces for failure analysis:%s", err)
		}
	}()
	// Restart with default parameter.
	defer secagentdupstart.RestartSecagentd(cleanupCtx)

	// Clear out old entries from the kernel trace to make an easier failure
	// analysis.
	if err := secagentdcommon.ClearKernelTrace(ctx); err != nil {
		s.Log("Unable to clear the kernel trace file: ", err)
	}

	// Restart chrome with the network event feature.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("CrOSLateBootSecagentdXDRNetworkEvents"))
	if err != nil {
		s.Fatal("Failed to restart chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	const batchIntervalS = 5
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))
	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	stop, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	if err := secagentdprocfsscraper.WaitForBpfMaps(ctx, agentPid); err != nil {
		s.Fatal("Failed to verify secagentd is ready to test: ", err)
	}

	netType := s.Param().(networkTypeParams).protocol
	netFam := s.Param().(networkTypeParams).family
	var svr *server
	var testEnv *routing.SimpleNetworkEnv
	var port = ""
	var addrStr = ""

	// Set up virtual network.
	testEnv, svr, err = setupL4server(ctx, netType, netFam)
	if err != nil {
		s.Fatal("Failed to setup router: ", err)
	}
	addrStr = svr.addr.String()
	port = strconv.Itoa(svr.port)
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	// Get details of sender.
	details, err := getNetworkProtocolDetails(ctx, netType, addrStr, port)
	if err != nil {
		s.Fatal("Fail to get NetworkProtocolDetails: ", err)
	}
	senderCmd := details.senderCmd
	receiverCmd := details.receiverCmd

	if receiverCmd != nil {
		if err := receiverCmd.Start(); err != nil {
			s.Fatalf("Error starting %q: %v ", receiverCmd, err)
		}
	}

	if err := senderCmd.Start(); err != nil {
		s.Fatalf("Error starting %q: %v ", senderCmd, err)
	}
	cmdPid := uint64(senderCmd.Process.Pid)
	s.Logf("Pid is %d", cmdPid)

	// Wait for the current batch to be flushed.
	// GoBigSleepLint: Using poll here doesn't make sense. There is no particular
	// condition we can poll for. This is simply giving secagentd ample time to
	// process and post events to dbus and is an educated guess.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, 2*batchIntervalS*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := senderCmd.Kill(); err != nil {
		s.Fatalf("Failed to kill %q: %v", senderCmd, err)
	}
	// Don't check the error here because it will likely just say
	// "signal: Killed"
	senderCmd.Wait()

	if receiverCmd != nil {
		if err := receiverCmd.Kill(); err != nil {
			s.Fatalf("Failed to kill %q: %v", receiverCmd, err)
		}
		receiverCmd.Wait()
	}

	// Collect the log of EnqueueRecord dbus calls to Missived.
	calledMethods, err := stop()
	if err != nil {
		s.Fatal("Failed to capture EnqueueRecord dbus calls to missived: ", err)
	}
	s.Logf("secagentd enqueued %d events", len(calledMethods))

	foundPid := false
	foundProtocol := false
	for _, method := range calledMethods {

		if len(method.Arguments) == 0 {
			continue
		}
		arg, ok := method.Arguments[0].([]byte)
		if !ok {
			continue
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			s.Fatal("Failed to unmarshal an EnqueueRecordRequest: ", err)
		}

		s.Logf("Destination is %s", enq.GetRecord().GetDestination())

		// Only focus on network event.
		if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_NETWORK {
			ne := &xdr.XdrNetworkEvent{}
			if err := proto.Unmarshal(enq.GetRecord().GetData(), ne); err != nil {
				s.Fatal("Failed to unmarshal data for a Destination_CROS_SECURITY_NETWORK record: ", err)
			}

			var bFlows []*xdr.NetworkFlowEvent

			for _, v := range ne.GetBatchedEvents() {
				if v.GetNetworkFlow() != nil {
					bFlows = append(bFlows, v.GetNetworkFlow())
				}

				if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
					s.Error("Invalid common field: ", err)
				}
			}

			for _, flow := range bFlows {
				if (flow.GetParentProcess() != nil && flow.GetParentProcess().GetCanonicalPid() == cmdPid) || (flow.GetProcess() != nil && flow.GetProcess().GetCanonicalPid() == cmdPid) {
					foundPid = true
					if flow.NetworkFlow.Protocol.String() == details.protocol && (details.ipAddr == "" || *flow.NetworkFlow.RemoteIp == details.ipAddr) {
						foundProtocol = true
						s.Logf("%s is captured", flow.NetworkFlow.Protocol.String())
					}
					s.Log(flow.String())
				}
			}
		}
		if foundPid && foundProtocol {
			break
		}
	}
	if !foundPid {
		s.Error("NetworkEvent is not captured")
	}

	if !foundProtocol {
		s.Errorf("Protocol %s is not captured", details.protocol)
	}
}

func setupL4server(ctx context.Context, network networkType, networkFam l4server.Family) (*routing.SimpleNetworkEnv, *server, error) {
	testEnv := routing.NewSimpleNetworkEnv(true, true, true, true)
	if err := testEnv.SetUp(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set up routing test env")
	}

	success := false
	defer func(ctx context.Context) {
		if success {
			return
		}
		if err := testEnv.TearDown(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to tear down routing test env in setupL4server: ", err)
		}
	}(ctx)

	// Wait for online and verify topology in host.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for service online")
	}
	routerAddrs, err := testEnv.Router.WaitForVethInAddrs(ctx, true, true)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get inner addrs from router env")
	}

	var pingAddrsV4 string
	var pingAddrsV6 []string

	// Add IPV4 Addresses.
	// Note: we can add DNS resolution too, but for MVP it's not necessary.
	pingAddrsV4 = routerAddrs.IPv4Addr.String()

	// Add IPV6 Addresses.
	for _, ip := range routerAddrs.IPv6Addrs {
		pingAddrsV6 = append(pingAddrsV6, ip.String())
	}
	pingAddrsV6 = append(pingAddrsV6, routing.TestDomainNameV6)

	// Ping and make sure connection has been established.
	for _, target := range pingAddrsV6 {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			return nil, nil, errors.Wrapf(err, "network verification failed: %v is not reachable as user %s on host", target, "chronos")
		}
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, pingAddrsV4, "chronos", 10*time.Second); err != nil {
		return nil, nil, errors.Wrapf(err, "network verification failed: %v is not reachable as user %s on host", pingAddrsV4, "chronos")

	}
	var addr net.IP

	if network == udpV6 || network == tcpV6 {
		addr = routerAddrs.IPv6Addrs[0]
	} else {
		addr = routerAddrs.IPv4Addr
	}
	port := 65535
	newServer := l4server.New(networkFam, port, l4server.WithAddr(addr.String()), l4server.WithMsgHandler(l4server.Reflector()))
	if err := testEnv.Router.StartServer(ctx, networkFam.String(), newServer); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to start %s server, error", networkFam)
	}
	svr := &server{
		port: port,
		addr: addr,
	}
	success = true

	return testEnv, svr, nil
}

func getNetworkProtocolDetails(ctx context.Context, network networkType, externIP, externPort string) (networkProtocolDetails, error) {
	switch network {
	case icmp:
		ipAddr := externIP
		cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("ping %s", ipAddr))

		return networkProtocolDetails{
			senderCmd:   cmd,
			receiverCmd: nil,
			protocol:    "ICMP",
			// TODO(jasonling): ICMP doesn't capture IP addr in the event.
			ipAddr: "",
		}, nil
	case tcp:
		cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, TCP\" | nc -v %s %s", externIP, externPort))

		return networkProtocolDetails{
			senderCmd:   cmd,
			receiverCmd: nil,
			protocol:    "TCP",
			ipAddr:      externIP,
		}, nil
	case tcpV6:
		senderCmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, TCP\" | nc -v -6 %s %s", externIP, externPort))

		return networkProtocolDetails{
			senderCmd:   senderCmd,
			receiverCmd: nil,
			protocol:    "TCP",
			ipAddr:      externIP,
		}, nil
	case udp:
		senderCmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, UDP\" | nc -u %s %s", externIP, externPort))

		return networkProtocolDetails{
			senderCmd:   senderCmd,
			receiverCmd: nil,
			protocol:    "UDP",
			ipAddr:      externIP,
		}, nil
	case udpV6:
		senderCmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, UDP\" | nc -6 -u %s %s", externIP, externPort))

		return networkProtocolDetails{
			senderCmd:   senderCmd,
			receiverCmd: nil,
			protocol:    "UDP",
			ipAddr:      externIP,
		}, nil
	}
	return networkProtocolDetails{}, errors.Errorf("An unexpected network type is received: %s", network)
}
