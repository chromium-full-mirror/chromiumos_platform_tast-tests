// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perfmanager

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/utils"
	remoteip "go.chromium.org/tast-tests/cros/remote/network/ip"
	iperf "go.chromium.org/tast-tests/cros/remote/network/iperf"
	remoteping "go.chromium.org/tast-tests/cros/remote/network/ping"
	routerSupport "go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// TestType is the type for perf test types.
type TestType string

// These are the different performance test types that are supported by
// Tast. They are defined from perspective of the Device Under Test, so for
// example 'tcp_rx' refers to a performance test of data transfer from a remote
// server to the DUT using the TCP protocol.
const (
	TestTypeTCPTx            TestType = "tcp_tx"
	TestTypeTCPRx            TestType = "tcp_rx"
	TestTypeTCPBidirectional TestType = "tcp_bidirectional"
	TestTypeUDPTx            TestType = "udp_tx"
	TestTypeUDPRx            TestType = "udp_rx"
	TestTypeUDPBidirectional TestType = "udp_bidirectional"
	TestTypeUDPTxSmall       TestType = "udp_tx_small"
	TestTypeUDPRxSmall       TestType = "udp_rx_small"
)

const (
	defaultRouterLANIPAddress = "192.168.1.50"
	defaultPcapLANIPAddress   = "192.168.1.51"
	defaultRouterLANIfaceName = "eth1"
	defaultPcapLANIfaceName   = "eth1"
)

var iperfProtocolMap = map[TestType]iperf.Protocol{
	TestTypeTCPTx:            iperf.ProtocolTCP,
	TestTypeTCPRx:            iperf.ProtocolTCP,
	TestTypeUDPTx:            iperf.ProtocolUDP,
	TestTypeUDPRx:            iperf.ProtocolUDP,
	TestTypeTCPBidirectional: iperf.ProtocolTCP,
	TestTypeUDPBidirectional: iperf.ProtocolUDP,
	TestTypeUDPTxSmall:       iperf.ProtocolUDP,
	TestTypeUDPRxSmall:       iperf.ProtocolUDP,
}

// TestManager is a helper class that manages running different types of iperf and netperf tests.
type TestManager struct {
	use2WaySetup bool
	routerType   routerSupport.RouterType
	router       *ssh.Conn
	routerIPAdd  string
	testDev      *ssh.Conn
	testDevIface string
	testDevIPAdd string
	peerDev      *ssh.Conn
	peerDevIPAdd string
	iperfClient  *iperf.RemoteClient
	iperfServer  *iperf.RemoteServer
}

// NewTestManager creates a new TestManager utility.
func NewTestManager(ctx context.Context, dutConn, routerConn, pcapConn *ssh.Conn, routerType routerSupport.RouterType, dutIP, routerIP, dutIface string) (*TestManager, error) {
	var use2WaySetup bool
	var peerDevice *ssh.Conn
	var peerDeviceIPAddress string
	if routerType == routerSupport.OpenWrtT || routerType == routerSupport.UbuntuT || routerType == routerSupport.SoftAPT {
		testing.ContextLog(ctx, "using 2-way setup")
		use2WaySetup = true
		peerDevice = routerConn
		peerDeviceIPAddress = routerIP
	} else {
		testing.ContextLog(ctx, "using 3-way setup")
		use2WaySetup = false
		if err := setup3WayIPConfig(ctx, dutConn, routerConn, pcapConn, dutIP, routerIP, dutIface); err != nil {
			return nil, err
		}
		peerDevice = pcapConn
		peerDeviceIPAddress = defaultPcapLANIPAddress
	}
	return &TestManager{
		use2WaySetup: use2WaySetup,
		routerType:   routerType,
		router:       routerConn,
		routerIPAdd:  routerIP,
		testDev:      dutConn,
		testDevIface: dutIface,
		testDevIPAdd: dutIP,
		peerDev:      peerDevice,
		peerDevIPAdd: peerDeviceIPAddress,
	}, nil
}

// setup3WayIPConfig sets up the IP configs required by the test for the 3-way-setup.
func setup3WayIPConfig(ctx context.Context, dutConn, routerConn, pcapConn *ssh.Conn, dutIP, routerIP, dutIface string) error {
	iprDut := remoteip.NewRemoteRunner(dutConn)
	iprRouter := remoteip.NewRemoteRunner(routerConn)
	iprPcap := remoteip.NewRemoteRunner(pcapConn)

	if err := iprRouter.SetLinkUp(ctx, defaultRouterLANIfaceName); err != nil {
		return err
	}
	if err := iprPcap.SetLinkUp(ctx, defaultPcapLANIfaceName); err != nil {
		return err
	}
	if err := iprRouter.FlushIP(ctx, defaultRouterLANIfaceName); err != nil {
		return err
	}
	if err := iprRouter.AddIP(ctx, defaultRouterLANIfaceName, net.ParseIP(defaultRouterLANIPAddress), 24); err != nil {
		return err
	}
	if err := iprPcap.FlushIP(ctx, defaultPcapLANIfaceName); err != nil {
		return err
	}
	if err := iprPcap.AddIP(ctx, defaultPcapLANIfaceName, net.ParseIP(defaultPcapLANIPAddress), 24); err != nil {
		return err
	}
	if err := iprDut.RouteIPVia(ctx, dutIface, net.ParseIP(defaultPcapLANIPAddress), net.ParseIP(routerIP)); err != nil {
		return err
	}
	if err := iprPcap.RouteIPVia(ctx, defaultRouterLANIfaceName, net.ParseIP(dutIP), net.ParseIP(defaultRouterLANIPAddress)); err != nil {
		return err
	}

	// Ensure that the router and pcap device in the cell have a direct
	// connection available over their respective LAN ports. Return an error
	// if this connection cannot be verified.
	pr := remoteping.NewRemoteRunner(routerConn)
	opts := []ping.Option{ping.BindAddress(true), ping.SourceIface(defaultRouterLANIfaceName)}
	res, err := pr.Ping(ctx, defaultPcapLANIPAddress, opts...)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "ping statistics=%+v", res)
	if res.Received == 0 {
		return errors.New("failed to verify connection between router and pcap devices. Router and pcap device must have a direct Ethernet connection over their LAN ports to run performance tests: No packets were received")
	}
	return nil
}

// Config returns iperf or netperf config based on the useIperf value and type of test.
func (p *TestManager) Config(routerType routerSupport.RouterType, testType TestType, testTime time.Duration) (*iperf.Config, error) {
	var options []iperf.ConfigOption
	if testTime > 0 {
		options = append(options, iperf.TestTimeOption(testTime))
	}
	// TODO(b/271490937): wifi_perf_openwrt: UDP TX perf numbers are low on
	// openwrt. Overwrite the num_ports for UDP_TX on openwrt since it's the
	// only viable workaround to improve the Tput in such case.
	if testType == TestTypeUDPTx && routerType == routerSupport.OpenWrtT {
		options = append(options, iperf.PortCountOption(1))
	}
	if testType == TestTypeUDPRxSmall || testType == TestTypeUDPTxSmall {
		options = append(options, iperf.DatagramLengthOption(112*iperf.B))
	}
	iperfTestType, ok := iperfProtocolMap[testType]
	if !ok {
		return nil, errors.Errorf("failed to find the iperf protocol for %s", testType)
	}

	if (testType == TestTypeTCPBidirectional) || (testType == TestTypeTCPTx) || (testType == TestTypeTCPRx) {
		options = append(options, iperf.ClientWindowSizeOption(320*iperf.KB))
		options = append(options, iperf.ServerWindowSizeOption(320*iperf.KB))
	} else if routerType == routerSupport.LegacyT {
		// 320kB is the maximum socket buffer size on Gale (default is 208kB).
		options = append(options, iperf.ServerWindowSizeOption(320*iperf.KB))
	}

	switch testType {
	case TestTypeTCPBidirectional:
		// TCP bidirectional results reported by the client seem to be less reliable
		// than server which still align with netperf and iperf3 results.
		//
		// Running with only a single thread seems to be more stable than parallel (b/331280995).
		options = append(options, iperf.BidirectionalOption(true),
			iperf.FetchServerResultsOption(true), iperf.PortCountOption(1))
		// For bidirectional, use DUT as the client since we're interested in the results
		// from the DUT's perspective.
		return iperf.NewConfig(iperfTestType, p.testDevIPAdd, p.peerDevIPAdd, options...)
	case TestTypeUDPBidirectional:
		// Just run single threaded for UDP, otherwise we see very high losses and inaccurate results.
		options = append(options, iperf.BidirectionalOption(true), iperf.PortCountOption(1))
		return iperf.NewConfig(iperfTestType, p.testDevIPAdd, p.peerDevIPAdd, options...)
	case TestTypeTCPTx, TestTypeUDPTx, TestTypeUDPTxSmall:
		return iperf.NewConfig(iperfTestType, p.testDevIPAdd, p.peerDevIPAdd, options...)
	case TestTypeTCPRx, TestTypeUDPRx, TestTypeUDPRxSmall:
		return iperf.NewConfig(iperfTestType, p.peerDevIPAdd, p.testDevIPAdd, options...)
	default:
		return nil, errors.Errorf("Test type %s is not supported by this test", testType)
	}
}

// Session returns an iperf or netperf session based on the useIperf value and type of test.
func (p *TestManager) Session(ctx context.Context, testType TestType) (*iperf.Session, error) {
	var err error
	switch testType {
	case TestTypeTCPTx, TestTypeUDPTx, TestTypeTCPBidirectional, TestTypeUDPBidirectional, TestTypeUDPTxSmall:
		p.iperfClient, err = iperf.NewRemoteClient(ctx, p.testDev)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Iperf client")
		}
		p.iperfServer, err = iperf.NewRemoteServer(ctx, p.peerDev)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Iperf server")
		}
		return iperf.NewSession(p.iperfClient, p.iperfServer), nil
	case TestTypeTCPRx, TestTypeUDPRx, TestTypeUDPRxSmall:
		p.iperfClient, err = iperf.NewRemoteClient(ctx, p.peerDev)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Iperf client")
		}
		p.iperfServer, err = iperf.NewRemoteServer(ctx, p.testDev)
		if err != nil {
			return nil, errors.Wrap(err, "failed ot create Iperf server")
		}
		return iperf.NewSession(p.iperfClient, p.iperfServer), nil
	default:
		return nil, errors.Errorf("Test type %s is not supported by this test", testType)
	}
}

// Close deconfigures all connections and routing.
func (p *TestManager) Close(ctx context.Context) error {
	if !p.use2WaySetup {
		if err := p.delete3WayIPConfig(ctx); err != nil {
			return err
		}
	}
	if p.iperfClient != nil {
		p.iperfClient.Close(ctx)
	}
	if p.iperfServer != nil {
		p.iperfServer.Close(ctx)
	}
	return nil
}

func (p *TestManager) delete3WayIPConfig(ctx context.Context) error {
	// Set up the IP configs required by the test for the 3-way-setup.
	iprDut := remoteip.NewRemoteRunner(p.testDev)
	iprRouter := remoteip.NewRemoteRunner(p.router)
	iprPcap := remoteip.NewRemoteRunner(p.peerDev)
	var firstErr error
	if err := iprDut.DeleteIPViaRoute(ctx, p.testDevIface, net.ParseIP(defaultPcapLANIPAddress), net.ParseIP(p.routerIPAdd)); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	if err := iprPcap.DeleteIPViaRoute(ctx, defaultRouterLANIfaceName, net.ParseIP(p.testDevIPAdd), net.ParseIP(defaultRouterLANIPAddress)); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	if err := iprRouter.DeleteIP(ctx, defaultRouterLANIfaceName, net.ParseIP(defaultRouterLANIPAddress), 24); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	if err := iprPcap.DeleteIP(ctx, defaultPcapLANIfaceName, net.ParseIP(defaultPcapLANIPAddress), 24); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	return firstErr
}
