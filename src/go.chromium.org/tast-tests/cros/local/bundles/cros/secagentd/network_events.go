// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"fmt"
	"net"
	"os/exec"
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
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

type networkProtocolDetails struct {
	cmds              []*testexec.Cmd
	protocol          xdr.NetworkProtocol
	expectedDirection xdr.NetworkFlow_Direction
	ipAddr            string
	pipeInText        string
}

type networkType string

type networkTypeParams struct {
	protocol     networkType
	family       l4server.Family
	isListenTest bool
	processCount uint
}

const (
	icmp        networkType = "ICMP"
	tcp         networkType = "TCP"
	tcpV6       networkType = "TCPV6"
	udp         networkType = "UDP"
	udpV6       networkType = "UDPV6"
	defaultPort int         = 65535
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
				isListenTest: false,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp",
			Val: networkTypeParams{
				protocol:     tcp,
				family:       l4server.TCP4,
				isListenTest: false,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp_v6",
			Val: networkTypeParams{
				protocol:     tcpV6,
				family:       l4server.TCP6,
				isListenTest: false,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp",
			Val: networkTypeParams{
				protocol:     udp,
				family:       l4server.UDP4,
				isListenTest: false,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "udp_v6",
			Val: networkTypeParams{
				protocol:     udpV6,
				family:       l4server.UDP6,
				isListenTest: false,
				processCount: 1,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "relaxed_icmp",
			Val: networkTypeParams{
				protocol:     icmp,
				family:       l4server.TCP4,
				isListenTest: false,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name: "relaxed_tcp",
			Val: networkTypeParams{
				protocol:     tcp,
				family:       l4server.TCP4,
				isListenTest: false,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name: "relaxed_tcp_v6",
			Val: networkTypeParams{
				protocol:     tcpV6,
				family:       l4server.TCP6,
				isListenTest: false,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name: "relaxed_udp",
			Val: networkTypeParams{
				protocol:     udp,
				family:       l4server.UDP4,
				isListenTest: false,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name: "relaxed_udp_v6",
			Val: networkTypeParams{
				protocol:     udpV6,
				family:       l4server.UDP6,
				isListenTest: false,
				processCount: 100,
			},
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		},
			{
				Name: "listen_tcp",
				Val: networkTypeParams{
					protocol:     tcp,
					family:       l4server.TCP,
					isListenTest: true,
					processCount: 1,
				},
				ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
			},
			{
				Name: "listen_tcp_v6",
				Val: networkTypeParams{
					protocol:     tcpV6,
					family:       l4server.TCP,
					isListenTest: true,
					processCount: 1,
				},
				ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
			},
		},
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
	defer secagentdupstart.RestartSecagentd(cleanupCtx, false)
	localAddrs := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			localAddrs[ipnet.IP.String()] = true
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
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))

	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	if err := secagentdprocfsscraper.WaitForBpfMaps(ctx, agentPid); err != nil {
		s.Fatal("Failed to verify secagentd is ready to test: ", err)
	}

	stopDbusMonitoring, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	netType := s.Param().(networkTypeParams).protocol
	netFam := s.Param().(networkTypeParams).family
	processCount := s.Param().(networkTypeParams).processCount
	isListenTest := s.Param().(networkTypeParams).isListenTest
	var testEnv *routing.SimpleNetworkEnv
	var addr *net.IP

	// Set up virtual network.
	testEnv, addr, err = setupTestNetwork(ctx, netType)
	if err != nil {
		s.Fatal("Failed to setup and verify test network")
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down test network: ", err)
		}
	}(cleanupCtx)
	var portStr = strconv.Itoa(defaultPort)
	var addrStr string
	if !isListenTest {
		// Install L4 server in the virtual network.
		addrStr = addr.String()
		err = setupL4server(ctx, netFam, testEnv, addr)
		if err != nil {
			s.Fatal("Failed to setup router: ", err)
		}
	} else {
		addrStr, err = getPrimaryDeviceAddress(ctx, netType)
		if err != nil {
			s.Fatal("Failed to determine listen address: ", err)
		}
	}

	// Get traffic generator (e.g nc, ping) or traffic receiver (e.g nc)
	// program details.
	details, err := getNetworkProtocolDetails(ctx, netType, addrStr, portStr, processCount, isListenTest)
	if err != nil {
		s.Fatal("Fail to get NetworkProtocolDetails: ", err)
	}

	var cmds []*testexec.Cmd = details.cmds
	cmdPids := make(map[uint64]bool)
	pidText := ""
	for _, cmd := range cmds {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			s.Fatalf("Unable to attach to the pipe of %q:%v", cmd.String(), err)
		}
		if err := cmd.Start(); err != nil {
			s.Fatalf("Error starting %q: %v ", cmd.String(), err)
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

	for _, cmd := range cmds {
		if err := cmd.Kill(); err != nil {
			s.Fatalf("Failed to kill %q: %v", cmd, err)
		}
	}
	for _, cmd := range cmds {
		// Don't check the error here because it will likely just say
		// "signal: Killed"
		cmd.Wait()
	}
	var bFlows []*xdr.NetworkFlowEvent
	var bListens []*xdr.NetworkSocketListenEvent
	var bExecs map[uint64]*xdr.ProcessExecEvent
	bExecs, bFlows, bListens, err = collectDbusMessages(ctx, s, cmdPids, stopDbusMonitoring)
	if err != nil {
		s.Fatalf("%v", err)
	}
	var logs []string
	if !isListenTest {
		logs, err = doNetworkFlowTest(&bFlows, &bExecs, &cmdPids, details, localAddrs)
	} else {
		logs, err = doNetworkListenTest(&bListens, &bExecs, &cmdPids, details)
	}
	for _, log := range logs {
		s.Log(log)
	}
	if err != nil {
		s.Fatal("test failed: ", err)
	}
}

func max(arg0, arg1 uint64) uint64 {
	if arg1 > arg0 {
		return arg1
	}
	return arg0
}

func min(arg0, arg1 uint64) uint64 {
	if arg1 < arg0 {
		return arg1
	}
	return arg0
}

func getPrimaryDeviceAddress(ctx context.Context, netType networkType) (string, error) {
	typeIsIpv4 := map[networkType]bool{icmp: true, tcp: true, udp: true,
		tcpV6: false, udpV6: false}
	var listenAddr string = ""

	/* use shill to grab the ipconfigs for the current active device. */
	manager, err := shill.NewManager(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to connect to shill manager")
	}
	service, err := manager.GetDefaultService(ctx)
	if err != nil {
		return "", errors.Wrap(err, "unable to retrieve default network service from shill manager")
	}
	device, err := service.GetDevice(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "unable to retrieve device from default service %q", service)
	}

	properties, err := device.GetProperties(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "unable to retrieve properties for the device %q of service %q", device, service)
	}
	ipconfigPaths, err := properties.GetObjectPaths("IPConfigs")
	if err != nil {
		return "", errors.Wrapf(err, "unable to retrieve IPConfigs from device %q of service %q", device, service)
	}

	if len(ipconfigPaths) != 2 { // only expect an ipv4 and ipv6 address
		return "", errors.Errorf("%d ip configs found for %q, expected two", len(ipconfigPaths), device)
	}

	for _, ifPath := range ipconfigPaths {
		ipConfig, err := shill.NewIPConfig(ctx, ifPath)
		if err != nil {
			return "", errors.Wrapf(err, "could not create an ip config from interface %q", ifPath)
		}
		ipConfigProperties, err := ipConfig.GetIPProperties(ctx)
		if err != nil {
			return "", errors.Wrapf(err, "unable to grab all properties from ipConfig %q", ipConfig)
		}
		listenAddr = ipConfigProperties.Address
		// make sure that the string parses into an ipv4 or ipv6 address.
		ip := net.ParseIP(listenAddr)
		if ip == nil {
			return "", errors.Errorf("%q is not a valid ip address", listenAddr)
		}
		if typeIsIpv4[netType] && ip.To4() != nil { // To4 succeeds on ipv4 address strings only.
			break
		} else if ip.To4() == nil {
			break
		}
	}
	return listenAddr, nil
}

func doNetworkListenTest(bListens *[]*xdr.NetworkSocketListenEvent, bExecs *map[uint64]*xdr.ProcessExecEvent, cmdPids *map[uint64]bool, details networkProtocolDetails) (
	[]string, error) {
	var log []string
	var err error
	log = append(log, fmt.Sprintf("Looking to match a socket listen with protocol=%s bind addr=%s bind port=%d",
		details.protocol, details.ipAddr, defaultPort))
	for _, listen := range *bListens {
		var matchPid *uint64 = nil
		matchPid, err = matchesProcess(listen.GetProcess(), cmdPids, bExecs)
		if matchPid != nil {
			if err != nil {
				log = append(log, fmt.Sprintf("flow process matching failed: %v", err))
			} else {
				err = verifyNetworkSocketListen(listen, details)
				if err != nil {
					log = append(log, fmt.Sprintf("Match failure: %v", err))
				} else {
					log = append(log, fmt.Sprintf("found a match: %s", listen.String()))
					return log, nil
				}
			}
		}
	}
	return log, errors.New("no socket listen matches found")
}

func doNetworkFlowTest(bFlows *[]*xdr.NetworkFlowEvent, bExecs *map[uint64]*xdr.ProcessExecEvent, cmdPids *map[uint64]bool, details networkProtocolDetails, localAddrs map[string]bool) ([]string, error) {
	// testing network flows.

	matchCount := 0
	processCount := len(*cmdPids)
	var err error = nil
	var txHigh, txLow, rxHigh, rxLow uint64 = 0, ^uint64(0), 0, ^uint64(0)
	badRemoteAddress := false

	var log []string
	for _, flow := range *bFlows {
		if localAddrs[flow.NetworkFlow.GetRemoteIp()] {
			log = append(log, fmt.Sprintf("Detected an event flow that has a local ip address as its remote address:%s Process: %s", flow.NetworkFlow.String(), flow.Process.String()))
			badRemoteAddress = true
		}
		var matchPid *uint64 = nil
		matchPid, err = matchesProcess(flow.GetProcess(), cmdPids, bExecs)
		if matchPid != nil {
			if err != nil {
				log = append(log, fmt.Sprintf("flow process matching failed: %v", err))
			} else {
				err = verifyNetworkFlow(flow, details)
				if err != nil {
					log = append(log, fmt.Sprintf("Match failure: %v", err))
				} else {
					matchCount++
					txHigh = max(*flow.NetworkFlow.TxBytes, txHigh)
					txLow = min(*flow.NetworkFlow.TxBytes, txLow)
					rxHigh = max(*flow.NetworkFlow.RxBytes, rxHigh)
					rxLow = min(*flow.NetworkFlow.RxBytes, rxLow)
				}
			}
			delete(*cmdPids, *matchPid)
		}
	}
	log = append(log, fmt.Sprintf("Matched %d/%d", matchCount, processCount))
	log = append(log, fmt.Sprintf("maxTx:%d minTx:%d maxRx:%d minRx:%d", txHigh, txLow, rxHigh, rxLow))
	if matchCount == 0 {
		return log, errors.Errorf("could not find a network flow event that matches expectations remote IP Address:%s protocol:%s direction:%s",
			details.ipAddr, details.protocol.String(), details.expectedDirection.String())
	}
	if badRemoteAddress {
		return log, errors.New("found one or more flows where the remote address in the flow is the same as a local ip address")
	}
	return log, nil
}

func verifyNetworkSocketListen(listen *xdr.NetworkSocketListenEvent, details networkProtocolDetails) error {
	var failedFields []string
	socket := listen.GetSocket()
	if socket.GetProtocol() != details.protocol {
		failedFields = append(failedFields, fmt.Sprintf("Protocol=%s expected %s", socket.GetProtocol(), details.protocol))
	}
	if socket.GetBindAddr() != details.ipAddr {
		failedFields = append(failedFields, fmt.Sprintf("IP Address=%q expected %q", socket.GetBindAddr(), details.ipAddr))
	}
	if socket.GetBindPort() != uint64(defaultPort) {
		failedFields = append(failedFields, fmt.Sprintf("Port=%d expected=%d", socket.GetBindPort(), defaultPort))
	}
	if len(failedFields) > 0 {
		return errors.Errorf("%s :%s", strings.Join(failedFields, ","), listen)
	}
	return nil
}

func verifyNetworkFlow(flow *xdr.NetworkFlowEvent, details networkProtocolDetails) error {
	var failedFields []string

	if *flow.NetworkFlow.Protocol != details.protocol {
		failedFields = append(failedFields, fmt.Sprintf("Protocol=%s expected %s", *flow.NetworkFlow.Protocol, details.protocol))
	}
	if details.ipAddr != "" && *flow.NetworkFlow.RemoteIp != details.ipAddr {
		failedFields = append(failedFields, fmt.Sprintf("IP Address=%q expected %q", *flow.NetworkFlow.RemoteIp, details.ipAddr))
	}
	if *flow.NetworkFlow.Direction != details.expectedDirection {
		failedFields = append(failedFields, fmt.Sprintf("Direction=%s expected %s", flow.NetworkFlow.Direction.String(), details.expectedDirection.String()))
	}
	if len(failedFields) > 0 {
		return errors.Errorf("%s :%s", strings.Join(failedFields, ","), flow)
	}
	return nil
}

func matchesProcess(processFromEvent *xdr.Process, cmdPids *map[uint64]bool, execMap *map[uint64]*xdr.ProcessExecEvent) (*uint64, error) {
	if processFromEvent != nil {
		pid := processFromEvent.GetCanonicalPid()
		if _, ok := (*cmdPids)[pid]; ok {
			if _, ok := (*execMap)[pid]; ok {
				// MetaFirstAppearance from an exec and a network event
				// are typically different. If you catch a process exec
				// then MetaFirstAppearance is probably true.
				// A process principle in a network event shouldn't even contain
				// MetaFirstAppearance.
				(*execMap)[pid].GetSpawnProcess().MetaFirstAppearance = nil
				processFromEvent.MetaFirstAppearance = nil
				if proto.Equal(processFromEvent, (*execMap)[pid].GetSpawnProcess()) {
					return &pid, nil
				}
				return &pid, errors.Errorf("PID matched but processes did not match: Expected %s, Actual:%s", (*execMap)[pid].GetSpawnProcess(), processFromEvent)
			}
			return &pid, nil // pid match but also no corresponding exec start.
		}
		return nil, nil //no pid match so not a process of interest.
	}
	return nil, errors.New("process to examine is nil")
}

func setupTestNetwork(ctx context.Context, network networkType) (*routing.SimpleNetworkEnv, *net.IP, error) {
	testEnv := routing.NewSimpleNetworkEnv(true, true, true, true)
	if err := testEnv.SetUp(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set up routing test env")
	}

	// Wait for online and verify topology in host.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for shill service to come online")
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
	return testEnv, &addr, nil
}

func setupL4server(ctx context.Context, networkFam l4server.Family, testEnv *routing.SimpleNetworkEnv, addr *net.IP) error {
	newServer := l4server.New(networkFam, defaultPort, l4server.WithAddr(addr.String()), l4server.WithMsgHandler(l4server.Reflector()))
	if err := testEnv.Router.StartServer(ctx, networkFam.String(), newServer); err != nil {
		return errors.Wrapf(err, "failed to start %s server, error", networkFam)
	}
	return nil
}

func getNetworkProtocolDetails(ctx context.Context, network networkType,
	externIP, externPort string, count uint, isListen bool) (networkProtocolDetails, error) {
	ncCmd, err := exec.LookPath("nc")
	if err != nil {
		return networkProtocolDetails{}, errors.Wrap(err, "unable to find nc command")
	}

	pingCmd, err := exec.LookPath("ping")
	if err != nil {
		return networkProtocolDetails{}, errors.Wrap(err, "unable to find ping command")
	}

	var cmd []*testexec.Cmd
	var pipeIn string = ""
	switch network {
	case icmp:
		ipAddr := externIP
		if isListen {
			cmd = nil
		} else {
			for i := uint(0); i < count; i++ {
				cmd = append(cmd, testexec.CommandContext(ctx, pingCmd, ipAddr))
			}
		}
		return networkProtocolDetails{
			cmds:              cmd,
			protocol:          xdr.NetworkProtocol_ICMP,
			expectedDirection: xdr.NetworkFlow_DIRECTION_UNKNOWN,
			ipAddr:            externIP,
			pipeInText:        pipeIn,
		}, nil
	case tcp:
		if isListen {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-l", "-s", externIP, "-p", externPort))
		} else {
			for i := uint(0); i < count; i++ {
				cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-v", externIP, externPort))
			}
			pipeIn = "Hello TCP"
		}
		return networkProtocolDetails{
			cmds:              cmd,
			protocol:          xdr.NetworkProtocol_TCP,
			expectedDirection: xdr.NetworkFlow_OUTGOING,
			ipAddr:            externIP,
			pipeInText:        pipeIn,
		}, nil
	case tcpV6:
		if isListen {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-l", "-6", "-s", externIP, "-p", externPort))
		} else {
			for i := uint(0); i < count; i++ {
				cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-6", externIP, externPort))
				pipeIn = "Hello TCP v6"
			}
		}
		return networkProtocolDetails{
			cmds:              cmd,
			protocol:          xdr.NetworkProtocol_TCP,
			expectedDirection: xdr.NetworkFlow_OUTGOING,
			ipAddr:            externIP,
			pipeInText:        pipeIn,
		}, nil
	case udp:
		if isListen {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-l", "-u", "-s", externIP, "-p", externPort))
		} else {
			for i := uint(0); i < count; i++ {
				cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-u", externIP, externPort))
				pipeIn = "Hello UDP"
			}

		}
		return networkProtocolDetails{
			cmds:              cmd,
			protocol:          xdr.NetworkProtocol_UDP,
			expectedDirection: xdr.NetworkFlow_DIRECTION_UNKNOWN, // UDP is directionless.
			ipAddr:            externIP,
			pipeInText:        pipeIn,
		}, nil
	case udpV6:
		if isListen {
			cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-l", "-u", "-6", "-s", externIP, "-p", externPort))
		} else {
			for i := uint(0); i < count; i++ {
				cmd = append(cmd, testexec.CommandContext(ctx, ncCmd, "-6", "-u", externIP, externPort))
			}
			pipeIn = "Hello UDPv6"
		}
		return networkProtocolDetails{
			cmds:              cmd,
			protocol:          xdr.NetworkProtocol_UDP,
			expectedDirection: xdr.NetworkFlow_DIRECTION_UNKNOWN, // UDP is directionless.
			ipAddr:            externIP,
			pipeInText:        pipeIn,
		}, nil
	}
	return networkProtocolDetails{}, errors.Errorf("An unexpected network type is received: %s", network)
}

func collectDbusMessages(ctx context.Context, s *testing.State,
	pids map[uint64]bool, stopDbusMonitoring func() ([]dbusutil.CalledMethod, error)) (
	map[uint64]*xdr.ProcessExecEvent, []*xdr.NetworkFlowEvent,
	[]*xdr.NetworkSocketListenEvent, error) {
	// Collect the log of EnqueueRecord dbus calls to Missived.
	calledMethods, err := stopDbusMonitoring()
	if err != nil {
		return nil, nil, nil,
			errors.Wrap(err, "failed to capture EnqueueRecord dbus calls to missived")
	}
	s.Logf("secagentd enqueued %d events", len(calledMethods))

	var bFlows []*xdr.NetworkFlowEvent
	var bListens []*xdr.NetworkSocketListenEvent
	bExecs := make(map[uint64]*xdr.ProcessExecEvent)
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
			return nil, nil, nil,
				errors.Wrap(err, "failed to unmarshal an EnqueueRecordRequest")
		}

		s.Logf("Destination is %s", enq.GetRecord().GetDestination())

		// Save off matching ProcessExecs.
		if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_PROCESS {
			pe := &xdr.XdrProcessEvent{}
			if err := proto.Unmarshal(enq.GetRecord().GetData(), pe); err != nil {
				return nil, nil, nil,
					errors.Wrap(err, "failed to unmarshal data for a CROS_SECURITY_PROCESS record")
			}
			for _, v := range pe.GetBatchedEvents() {
				if v.GetProcessExec() != nil {
					pid := v.GetProcessExec().GetSpawnProcess().GetCanonicalPid()
					if _, ok := pids[pid]; ok {
						bExecs[pid] = v.GetProcessExec()
						if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
							errors.Wrap(err, "CROS_SECURITY_PROCESS process exec has an invalid common field")
						}
					}
				}
			}
		}
		// Save off network event data.
		networkEventType := ""
		if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_NETWORK {
			ne := &xdr.XdrNetworkEvent{}
			if err := proto.Unmarshal(enq.GetRecord().GetData(), ne); err != nil {
				return nil, nil, nil,
					errors.Wrap(err, "failed to unmarshal data for a CROS_SECURITY_NETWORK record")
			}
			for _, v := range ne.GetBatchedEvents() {
				if v.GetNetworkFlow() != nil {
					bFlows = append(bFlows, v.GetNetworkFlow())
					networkEventType = "Network Flow"
				}
				if v.GetNetworkSocketListen() != nil {
					bListens = append(bListens, v.GetNetworkSocketListen())
					networkEventType = "Network Socket Listen"
				}
				if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
					errors.Wrapf(err, "CROS_SECURITY_NETWORK %v has an invalid common field", networkEventType)
				}
			}
		}
	}
	return bExecs, bFlows, bListens, nil
}
