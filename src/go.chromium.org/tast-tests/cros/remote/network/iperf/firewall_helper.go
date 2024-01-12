// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/network/firewall"
	fwremote "go.chromium.org/tast-tests/cros/remote/network/firewall"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
)

// firewallHelper adds firewall rules and keeps track of them for cleaning up later.
type firewallHelper struct {
	fwr          *fwremote.Runner
	cleanupRules [][]firewall.RuleOption
}

func newFirewallHelper(conn *ssh.Conn) *firewallHelper {
	return &firewallHelper{
		fwr:          fwremote.NewRemoteRunner(conn),
		cleanupRules: make([][]firewall.RuleOption, 0),
	}
}

func (f *firewallHelper) open(ctx context.Context, peerAddr string) error {
	// TODO(b/319776793): use static chain for test iptables rule.
	allowRxOpts := []firewall.RuleOption{
		firewall.OptionSource(peerAddr),
		firewall.OptionJumpTarget(firewall.TargetAccept),
		firewall.OptionWait(10),
	}
	allowTxOpts := []firewall.RuleOption{
		firewall.OptionDestination(peerAddr),
		firewall.OptionJumpTarget(firewall.TargetAccept),
		firewall.OptionWait(10),
	}
	rules := [][]firewall.RuleOption{
		// Command: iptables -I INPUT -s ${peerAddr} -j ACCEPT -w 10
		append([]firewall.RuleOption{firewall.OptionInsertRule(firewall.InputChain)}, allowRxOpts...),
		// Command: iptables -I OUTPUT -d ${peerAddr} -j ACCEPT -w 10
		append([]firewall.RuleOption{firewall.OptionInsertRule(firewall.OutputChain)}, allowTxOpts...),
	}
	f.cleanupRules = [][]firewall.RuleOption{
		// Command: iptables -D INPUT -s ${peerAddr} -j ACCEPT -w 10
		append([]firewall.RuleOption{firewall.OptionDeleteRule(firewall.InputChain)}, allowRxOpts...),
		// Command: iptables -D OUTPUT -d ${peerAddr} -j ACCEPT -w 10
		append([]firewall.RuleOption{firewall.OptionDeleteRule(firewall.OutputChain)}, allowTxOpts...),
	}

	var allErrors error
	for _, fw := range rules {
		if err := f.fwr.ExecuteCommand(ctx, fw...); err != nil {
			allErrors = errors.Wrapf(allErrors, "failed to configure firewall, %s", err) // NOLINT
		}
	}

	if allErrors != nil {
		f.close(ctx)
	}

	return allErrors
}

func (f *firewallHelper) close(ctx context.Context) error {
	// TODO(b/319776793): clean up static chain for test iptables rule.
	var allErrors error
	for _, fw := range f.cleanupRules {
		if err := f.fwr.ExecuteCommand(ctx, fw...); err != nil {
			allErrors = errors.Wrapf(allErrors, "failed to configure firewall, %s", err) // NOLINT
		}
	}

	f.cleanupRules = f.cleanupRules[:0]
	return allErrors
}
