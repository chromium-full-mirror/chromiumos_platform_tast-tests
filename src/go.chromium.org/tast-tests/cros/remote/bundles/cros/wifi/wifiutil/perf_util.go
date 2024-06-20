// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/network/firewall"
	"go.chromium.org/tast-tests/cros/common/utils"
	remotefirewall "go.chromium.org/tast-tests/cros/remote/network/firewall"
	"go.chromium.org/tast-tests/cros/remote/network/iperf"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast/core/errors"
)

// P2PPerf pings the p2p client from the group owner (GO) device.
func P2PPerf(ctx context.Context, p2pGO, p2pClient wificell.P2PWiFiDevice) (*iperf.Result, error) {
	// p2pGOFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p GO.
	var p2pGOFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionAppendRule(firewall.InputChain),
		firewall.OptionSource(utils.P2PGOIPAddress),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	// p2pClientFirewallParams is a set of parameters needed for unblocking p2p tcp traffic on the p2p client.
	var p2pClientFirewallParams = []firewall.RuleOption{
		firewall.OptionWait(5),
		firewall.OptionAppendRule(firewall.InputChain),
		firewall.OptionSource(utils.P2PClientIPAddress),
		firewall.OptionProto(firewall.L4ProtoTCP),
		firewall.OptionMatch(firewall.L4ProtoTCP),
		firewall.OptionJumpTarget(firewall.TargetAccept),
	}

	firewallRunnerGO := remotefirewall.NewRemoteRunner(p2pGO.Conn())
	firewallRunnerClient := remotefirewall.NewRemoteRunner(p2pClient.Conn())
	if err := firewallRunnerGO.ExecuteCommand(ctx, p2pGOFirewallParams...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P GO iptable rule")
	}
	if err := firewallRunnerClient.ExecuteCommand(ctx, p2pClientFirewallParams...); err != nil {
		return nil, errors.Wrap(err, "failed to set P2P Client iptable rule")
	}

	// Configuring the p2p GO as an iperf server and the p2p client as an iperf client.
	p2pIperfConfig, err := iperf.NewConfig(iperf.ProtocolTCP, utils.P2PClientIPAddress, utils.P2PGOIPAddress, []iperf.ConfigOption{}...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure iperf on the p2p link")
	}

	client, err := iperf.NewRemoteClient(ctx, p2pClient.Conn())
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf client")
	}
	defer client.Close(ctx)

	server, err := iperf.NewRemoteServer(ctx, p2pGO.Conn())
	if err != nil {
		return nil, errors.Wrap(err, "failed ot create Iperf server")
	}
	defer server.Close(ctx)

	session := iperf.NewSession(client, server)

	finalResult, _, err := session.Run(ctx, p2pIperfConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run Iperf session")
	}

	return finalResult, nil
}
