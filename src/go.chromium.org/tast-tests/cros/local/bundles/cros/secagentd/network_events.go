// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	rep "chromiumos/reporting"
	xdr "chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
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

const (
	icmp  networkType = "ICMP"
	tcp   networkType = "TCP"
	tcpV6 networkType = "TCPV6"
	udp   networkType = "UDP"
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
			Name:      "icmp",
			Val:       icmp,
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name:      "tcp",
			Val:       tcp,
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tcp_v6",
			Val:  tcpV6,
		}, {
			Name:      "udp",
			Val:       udp,
			ExtraAttr: []string{"group:mainline", "informational"},
		}},
	})
}

// NetworkEvents trigger network events,
// and verifies it against the events emitted by secagentd over dbus.
func NetworkEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()
	// Restart with default parameter.
	defer secagentdupstart.RestartSecagentd(cleanupCtx)

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
	details, err := getNetworkProtocolDetails(ctx, s.Param().(networkType))
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

			foundPid := false
			foundProtocol := false
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

			if !foundPid {
				s.Error("NetworkEvent is not captured")
			}

			if !foundProtocol {
				s.Errorf("Protocol %s is not captured", details.protocol)
			}
		}
	}
}

func getNetworkProtocolDetails(ctx context.Context, network networkType) (networkProtocolDetails, error) {
	switch network {
	case icmp:
		// 8.8.8.8 is google DNS IPv4 address.
		ipAddr := "8.8.8.8"
		cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("ping %s", ipAddr))

		return networkProtocolDetails{
			senderCmd:   cmd,
			receiverCmd: nil,
			protocol:    "ICMP",
			// TODO(jasonling): ICMP doesn't capture IP addr in the event.
			ipAddr: "",
		}, nil
	case tcp:
		out, err := testexec.CommandContext(ctx, "sh", "-c", "dig www.google.com +short | head -1").Output()
		if err != nil {
			return networkProtocolDetails{}, errors.Wrap(err, "fail to get IP address of www.google.com")
		}

		ipAddr := strings.TrimSuffix(string(out), "\n")
		cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("wget -P /tmp %s", ipAddr))

		return networkProtocolDetails{
			senderCmd:   cmd,
			receiverCmd: nil,
			protocol:    "TCP",
			ipAddr:      ipAddr,
		}, nil
	case tcpV6:
		// 2001:4860:4860::8888 is google DNS IPv6 address.
		ipAddr := "fe80::f6f5:e8ff:fe50:eadc%eth0"
		cmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, TCP\" | nc -6 %s 53", ipAddr))

		return networkProtocolDetails{
			senderCmd:   cmd,
			receiverCmd: nil,
			protocol:    "TCP",
			ipAddr:      ipAddr,
		}, nil
	case udp:
		// Choose the account to use based on the IP address of the chromebook.
		cmdStr := `ifconfig eth0 | grep "inet " | awk '{print $2}'`
		out, err := testexec.CommandContext(ctx, "sh", "-c", cmdStr).Output()
		if err != nil {
			return networkProtocolDetails{}, errors.Wrap(err, "fail to get IPv4 address")
		}

		ipAddr := strings.TrimSuffix(string(out), "\n")

		senderCmd := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("echo \"Hello, UDP\" | nc -u %s 8080", ipAddr))
		receiverCmd := testexec.CommandContext(ctx, "sh", "-c", "nc -u -l -p 8080")

		return networkProtocolDetails{
			senderCmd:   senderCmd,
			receiverCmd: receiverCmd,
			protocol:    "UDP",
			ipAddr:      ipAddr,
		}, nil
	}
	return networkProtocolDetails{}, errors.Errorf("An unexpected network type is received: %s", network)
}
