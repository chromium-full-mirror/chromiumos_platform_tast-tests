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
	"strings"
	"time"

	rep "go.chromium.org/chromiumos/reporting"
	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
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
	senderCmds        []*testexec.Cmd
	receiverCmd       *testexec.Cmd
	protocol          xdr.NetworkProtocol
	expectedDirection xdr.NetworkFlow_Direction
	ipAddr            string
	pipeInText        string
}

type networkType string

type networkTypeParams struct {
	protocol     networkType
	family       l4server.Family
	processCount uint
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
				protocol:     icmp,
				family:       l4server.TCP4,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp",
			Val: networkTypeParams{
				protocol:     tcp,
				family:       l4server.TCP4,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp_v6",
			Val: networkTypeParams{
				protocol:     tcpV6,
				family:       l4server.TCP6,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp",
			Val: networkTypeParams{
				protocol:     udp,
				family:       l4server.UDP4,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp_v6",
			Val: networkTypeParams{
				protocol:     udpV6,
				family:       l4server.UDP6,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "relaxed_icmp",
			Val: networkTypeParams{
				protocol:     icmp,
				family:       l4server.TCP4,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
		}, {
			Name: "relaxed_tcp",
			Val: networkTypeParams{
				protocol:     tcp,
				family:       l4server.TCP4,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
		}, {
			Name: "relaxed_tcp_v6",
			Val: networkTypeParams{
				protocol:     tcpV6,
				family:       l4server.TCP6,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
		}, {
			Name: "relaxed_udp",
			Val: networkTypeParams{
				protocol:     udp,
				family:       l4server.UDP4,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
		}, {
			Name: "relaxed_udp_v6",
			Val: networkTypeParams{
				protocol:     udpV6,
				family:       l4server.UDP6,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "informational", "group:criticalstaging"},
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

	localAddress := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			localAddress[ipnet.IP.String()] = true
		}
	}
	// Clear out old entries from the kernel trace to make an easier failure
	// analysis.
	if err := secagentdcommon.ClearKernelTrace(ctx); err != nil {
		s.Log("Unable to clear the kernel trace file: ", err)
	}

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

	if err := secagentdprocfsscraper.WaitForBpfMaps(ctx, agentPid); err != nil {
		s.Fatal("Failed to verify secagentd is ready to test: ", err)
	}

	stop, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	netType := s.Param().(networkTypeParams).protocol
	netFam := s.Param().(networkTypeParams).family
	processCount := s.Param().(networkTypeParams).processCount
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
	details, err := getNetworkProtocolDetails(ctx, netType, addrStr, port, processCount)
	if err != nil {
		s.Fatal("Fail to get NetworkProtocolDetails: ", err)
	}

	senderCmds := details.senderCmds
	receiverCmd := details.receiverCmd

	if receiverCmd != nil {
		if err := receiverCmd.Start(); err != nil {
			s.Fatalf("Error starting %q: %v ", receiverCmd, err)
		}
	}

	cmdPids := make(map[uint64]bool)
	pidText := ""
	for _, cmd := range senderCmds {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			s.Fatalf("Unable to attach to the pipe of %q:%v", cmd.String(), err)
		}
		if err := cmd.Start(); err != nil {
			s.Fatalf("Error starting %q: %v ", senderCmds, err)
		}
		stdin.Write([]byte(details.pipeInText))
		pid := uint64(cmd.Process.Pid)
		cmdPids[pid] = true
		pidText += strconv.FormatUint(pid, 10) + ","
		stdin.Close()
	}
	pidText = strings.TrimSuffix(pidText, ",")
	s.Logf("pids=[%s]", pidText)

	// Wait for the current batch to be flushed.
	// GoBigSleepLint: Using poll here doesn't make sense. There is no particular
	// condition we can poll for. This is simply giving secagentd ample time to
	// process and post events to dbus and is an educated guess.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, 2*batchIntervalS*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	for _, cmd := range senderCmds {
		if err := cmd.Kill(); err != nil {
			s.Fatalf("Failed to kill %q: %v", senderCmds, err)
		}
	}
	for _, cmd := range senderCmds {
		// Don't check the error here because it will likely just say
		// "signal: Killed"
		cmd.Wait()
	}

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

	badRemoteAddress := false
	matchCount := 0
	var failedFields []string

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

			pidFound := false
			for _, flow := range bFlows {
				failedFields = nil
				pidFound = false
				if localAddress[flow.NetworkFlow.GetRemoteIp()] {
					s.Log("Detected an event flow that has a local ip address as its remote address:", flow.NetworkFlow.String())
					badRemoteAddress = true
				}
				if flow.GetProcess() != nil {
					if _, ok := cmdPids[flow.GetProcess().GetCanonicalPid()]; ok {
						pidFound = true
						delete(cmdPids, flow.GetProcess().GetCanonicalPid())
					}
				}
				if pidFound {
					if *flow.NetworkFlow.Protocol != details.protocol {
						failedFields = append(failedFields, fmt.Sprintf("Protocol=%s expected %s", *flow.NetworkFlow.Protocol, details.protocol))
					}
					if details.ipAddr != "" && *flow.NetworkFlow.RemoteIp != details.ipAddr {
						failedFields = append(failedFields, fmt.Sprintf("IP Address=%q expected %q", *flow.NetworkFlow.RemoteIp, details.ipAddr))
					}
					if *flow.NetworkFlow.Direction != details.expectedDirection {
						failedFields = append(failedFields, fmt.Sprintf("Direction=%s expected %s", flow.NetworkFlow.Direction.String(), details.expectedDirection.String()))
					}
					if len(failedFields) == 0 {
						matchCount++
					} else {
						s.Logf("Match failure:%s :%s", strings.Join(failedFields, ","), flow)
					}
				}
			}
		}
	}
	if badRemoteAddress {
		s.Error("Found one or more flows where the remote address in the flow is the same as a local ip address")
	}
	if matchCount == 0 {
		s.Errorf("Could not find a network flow event that matches expectations pid:%s remote IP Address:%s protocol:%s direction:%s",
			pidText, details.ipAddr, details.protocol.String(), details.expectedDirection.String())
	}
	s.Logf("Matched %d/%d", matchCount, processCount)
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

func getNetworkProtocolDetails(ctx context.Context, network networkType,
	externIP, externPort string, count uint) (networkProtocolDetails, error) {
	const ncCmd = "/usr/local/bin/nc"

	switch network {
	case icmp:
		ipAddr := externIP
		var cmd []*testexec.Cmd
		for i := uint(0); i < count; i++ {
			cmd = append(cmd, testexec.CommandContext(ctx, "/bin/ping", ipAddr))
		}
		return networkProtocolDetails{
			senderCmds:        cmd,
			receiverCmd:       nil,
			protocol:          xdr.NetworkProtocol_ICMP,
			expectedDirection: xdr.NetworkFlow_DIRECTION_UNKNOWN,
			ipAddr:            externIP,
			pipeInText:        "",
		}, nil
	case tcp:
		var cmd []*testexec.Cmd
		for i := uint(0); i < count; i++ {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-v", externIP, externPort))
		}
		return networkProtocolDetails{
			senderCmds:        cmd,
			receiverCmd:       nil,
			protocol:          xdr.NetworkProtocol_TCP,
			expectedDirection: xdr.NetworkFlow_OUTGOING,
			ipAddr:            externIP,
			pipeInText:        "Hello, TCP",
		}, nil
	case tcpV6:
		var cmd []*testexec.Cmd
		for i := uint(0); i < count; i++ {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-6", externIP, externPort))
		}
		return networkProtocolDetails{
			senderCmds:        cmd,
			receiverCmd:       nil,
			protocol:          xdr.NetworkProtocol_TCP,
			expectedDirection: xdr.NetworkFlow_OUTGOING,
			ipAddr:            externIP,
			pipeInText:        "Hello TCPv6",
		}, nil
	case udp:
		var cmd []*testexec.Cmd
		for i := uint(0); i < count; i++ {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-u", externIP, externPort))
		}
		return networkProtocolDetails{
			senderCmds:        cmd,
			receiverCmd:       nil,
			protocol:          xdr.NetworkProtocol_UDP,
			expectedDirection: xdr.NetworkFlow_DIRECTION_UNKNOWN,
			ipAddr:            externIP,
			pipeInText:        "Hello UDP",
		}, nil
	case udpV6:
		var cmd []*testexec.Cmd
		for i := uint(0); i < count; i++ {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-6", "-u", externIP, externPort))
		}
		return networkProtocolDetails{
			senderCmds:        cmd,
			receiverCmd:       nil,
			protocol:          xdr.NetworkProtocol_UDP,
			expectedDirection: xdr.NetworkFlow_OUTGOING,
			ipAddr:            externIP,
			pipeInText:        "Hello UDPv6",
		}, nil
	}
	return networkProtocolDetails{}, errors.Errorf("An unexpected network type is received: %s", network)
}
