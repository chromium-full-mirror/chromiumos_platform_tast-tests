// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package setup

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// ifconfigRE parses one adapter from the output of ifconfig.
	ifconfigRE    = regexp.MustCompile("([^:]+): .*\n(?: +.*\n)*\n")
	multicastRE   = regexp.MustCompile("flags=.*<.*(ALLMULTI|MULTICAST).*>")
	ethProperties = map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeEthernet,
		shillconst.ServicePropertyIsConnected: true,
	}
)

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

// DisableAllMulticast disables multicast on all ethernet and wlan interfaces.
func DisableAllMulticast(ctx context.Context) (CleanupCallback, error) {
	return Nested(ctx, "disable network multicast", func(s *Setup) error {
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
		return nil
	})
}

// IsEthernetConnected returns true if the ethernet service is connected.
func IsEthernetConnected(ctx context.Context) bool {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create a shill manager: ", err)
		return false
	}

	if _, err := manager.FindMatchingService(ctx, ethProperties); err != nil {
		testing.ContextLog(ctx, "Failed to find ethernet service: ", err)
		return false
	}
	return true
}

// waitForEthernet waits for the ethernet service.
func waitForEthernet(ctx context.Context) error {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a shill manager")
	}

	testing.ContextLog(ctx, "Waiting for an Ethernet Service")
	start := time.Now()
	_, err = manager.WaitForServiceProperties(ctx, ethProperties, 30*time.Second)
	if err != nil {
		return errors.Wrap(err, "failed to wait for an ethernet service")
	}
	testing.ContextLog(ctx, "Wait for ethernet took: ", time.Since(start))
	return nil
}

// ensureEthernetConnectedFor ensures that Ethernet remains connected continuously
// for at least the specified duration within the given timeout period.
func ensureEthernetConnectedFor(ctx context.Context, duration, timeout time.Duration) error {
	if err := waitForEthernet(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for an ethernet service")
	}
	timer := time.Now()
	return testing.Poll(ctx, func(ctx context.Context) error {
		if time.Since(timer) >= duration {
			return nil
		}
		if !IsEthernetConnected(ctx) {
			// Reset timer until the ethernet gets connected.
			timer = time.Now()
			return errors.New("ethernet is not connected")
		}
		return errors.Errorf("still waiting for the ethernet connected for %v", duration)
	}, &testing.PollOptions{Interval: time.Second, Timeout: timeout})
}
