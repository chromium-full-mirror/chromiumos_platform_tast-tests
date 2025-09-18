// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ifconfigRE parses one adapter from the output of ifconfig.
var ifconfigRE = regexp.MustCompile("([^:]+): .*\n(?: +.*\n)*\n")
var multicastRE = regexp.MustCompile("flags=.*<.*(ALLMULTI|MULTICAST).*>")

func listUpNetworkInterfaces(ctx context.Context) ([]string, error) {
	output, err := testexec.CommandContext(ctx, "ifconfig").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get interface list")
	}
	var interfaces []string
	match := ifconfigRE.FindAllSubmatch(output, -1)
	if match == nil {
		return nil, errors.Errorf("unable to parse interface list from %q", output)
	}
	for _, submatch := range match {
		interfaces = append(interfaces, string(submatch[1]))
	}
	return interfaces, nil
}

func listMulticastOnInterfaces(ctx context.Context) ([]string, error) {
	output, err := testexec.CommandContext(ctx, "ifconfig").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get interface list")
	}
	var multicastInterfaces []string
	match := ifconfigRE.FindAllSubmatch(output, -1)
	if match == nil {
		return nil, errors.Errorf("unable to parse interface list from %q", output)
	}
	for _, submatch := range match {
		if multicastRE.MatchString(string(submatch[0])) {
			multicastInterfaces = append(multicastInterfaces, string(submatch[1]))
		}
	}
	return multicastInterfaces, nil
}

func enableNetworkInterface(ctx context.Context, iface string) error {
	if err := testexec.CommandContext(ctx, "ifconfig", iface, "up").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to enable network interface %q", iface)
	}
	return nil
}

func disableNetworkInterface(ctx context.Context, iface string) error {
	if err := testexec.CommandContext(ctx, "ifconfig", iface, "down").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to disable network interface %q", iface)
	}
	return nil
}

// DisableNetworkInterface disables a single network interface.
func DisableNetworkInterface(ctx context.Context, iface string) (CleanupCallback, error) {
	testing.ContextLogf(ctx, "Disabling network interface %q", iface)
	if err := disableNetworkInterface(ctx, iface); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Re-enabling network interface %q", iface)
		return enableNetworkInterface(ctx, iface)
	}, nil
}

// DisableNetworkInterfaces disables all network interfaces whose names match a
// regexp.
func DisableNetworkInterfaces(ctx context.Context, pattern *regexp.Regexp) (CleanupCallback, error) {
	return Nested(ctx, "disable network interface", func(s *Setup) error {
		upInterfaces, err := listUpNetworkInterfaces(ctx)
		if err != nil {
			return err
		}

		for _, iface := range upInterfaces {
			if !pattern.MatchString(iface) {
				continue
			}
			s.Add(DisableNetworkInterface(ctx, iface))
		}
		return nil
	})
}

// DisableWiFiAdaptors disables all WiFi adapters and returns a callback to
// re-enable them.
func DisableWiFiAdaptors(ctx context.Context) (CleanupCallback, error) {
	var wifiInterfacePattern = regexp.MustCompile(`.*wlan\d+`)
	return DisableNetworkInterfaces(ctx, wifiInterfacePattern)
}

func enableNetworkMulticast(ctx context.Context, iface string) error {
	if err := testexec.CommandContext(ctx, "ifconfig", iface, "multicast", "allmulti").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to enable multicast on network interface %q", iface)
	}
	return nil
}

func disableNetworkMulticast(ctx context.Context, iface string) error {
	if err := testexec.CommandContext(ctx, "ifconfig", iface, "-multicast", "-allmulti").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "unable to disable multicast on network interface %q", iface)
	}
	return nil
}

// DisableNetworkMulticast disables multicast on a single network interface
func DisableNetworkMulticast(ctx context.Context, iface string) (CleanupCallback, error) {
	testing.ContextLogf(ctx, "Disabling multicast on network interface %q", iface)
	if err := disableNetworkMulticast(ctx, iface); err != nil {
		return nil, err
	}

	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Re-enabling multicast on network interface %q", iface)
		// Other power setups (suspect toggling wifi) could cause a usb re-connect.
		// As a result, "eth0" via usb dongles could be temporarily not available
		// for 5-6s. Using a poll to work around the problem.  See b/262254989.
		return testing.Poll(ctx, func(ctx context.Context) error {
			return enableNetworkMulticast(ctx, iface)
		}, &testing.PollOptions{
			Timeout:  15 * time.Second,
			Interval: 5 * time.Second,
		})
	}, nil
}

// BlockNetworkTrafficsWithIPTables blocks network traffics with |iptables| and |ip6tables|.
func BlockNetworkTrafficsWithIPTables(ctx context.Context) (CleanupCallback, error) {
	var (
		blockRules = [][]string{
			// Rules to block multicast traffics.
			{"iptables", "-I", "INPUT", "-s", "224.0.0.0/4", "-j", "DROP", "-w"},
			{"iptables", "-I", "INPUT", "-d", "224.0.0.0/4", "-j", "DROP", "-w"},
			{"ip6tables", "-I", "INPUT", "-s", "ff00::/8", "-j", "DROP", "-w"},
			{"ip6tables", "-I", "INPUT", "-d", "ff00::/8", "-j", "DROP", "-w"},
			// Rules to block NetBIOS traffics.
			{"iptables", "-I", "INPUT", "-i", "eth0", "-p", "udp", "--dport", "137:139", "-j", "DROP", "-w"},
			{"iptables", "-I", "INPUT", "-i", "eth0", "-p", "tcp", "--dport", "137:139", "-j", "DROP", "-w"},
			// Rule to block ICMPv6 traffic.
			{"ip6tables", "-I", "INPUT", "-i", "eth0", "-p", "icmpv6", "--icmpv6-type", "143", "-j", "DROP", "-w"},
		}
		unblockRules [][]string
	)

	cleanup := func(ctx context.Context) error {
		var firstErr error
		testing.ContextLog(ctx, "Unblocking network traffics, rule counts: ", len(unblockRules))
		for _, unblockRule := range unblockRules {
			if err := testexec.CommandContext(ctx, unblockRule[0], unblockRule[1:]...).Run(testexec.DumpLogOnError); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				testing.ContextLogf(ctx, "Unable to remove rule: %v: %v", unblockRule, err)
			}
		}
		return firstErr
	}

	testing.ContextLog(ctx, "Blocking network traffics")
	for _, blockRule := range blockRules {
		if err := testexec.CommandContext(ctx, blockRule[0], blockRule[1:]...).Run(testexec.DumpLogOnError); err != nil {
			return cleanup, errors.Wrapf(err, "unable to apply rule: %v", blockRule)
		}
		// Replace the insert "-I" with delete "-D" to remove the rule.
		unblockRule := append([]string{blockRule[0], "-D"}, blockRule[2:]...)
		unblockRules = append(unblockRules, unblockRule)
	}
	return cleanup, nil
}

// DisableAllMulticast disables multicast on all ethernet and wlan interfaces.
func DisableAllMulticast(ctx context.Context) (CleanupCallback, error) {
	return Nested(ctx, "disable ntwork multicast", func(s *Setup) error {
		pattern := regexp.MustCompile("(eth|wlan).*")
		onInterfaces, err := listMulticastOnInterfaces(ctx)
		if err != nil {
			return err
		}

		for _, iface := range onInterfaces {
			if !pattern.MatchString(iface) {
				continue
			}
			s.Add(DisableNetworkMulticast(ctx, iface))
		}
		s.Add(BlockNetworkTrafficsWithIPTables(ctx))
		return nil
	})
}
