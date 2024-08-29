// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc provides ARC-related networking functionality.
package arc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	pp "go.chromium.org/chromiumos/system_api/patchpanel_proto"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ExpectPingSuccess checks if 'addr' is reachable over the 'network' in ARC.
// See ArcNetworkDebugTools#reachCmd for possible 'network' values.
// Use an empty 'network' to test on default network.
func ExpectPingSuccess(ctx context.Context, a *arc.ARC, network, addr string) error {
	if network == "" {
		testing.ContextLogf(ctx, "Start to ping %s from ARC over default network", addr)
	} else {
		testing.ContextLogf(ctx, "Start to ping %s from ARC over %q", addr, network)
	}
	// This polls for 20 seconds before it gives up on pinging from within ARC. We
	// poll for a little bit since the ARP table within ARC might not be populated
	// yet - so give it some time before the ping makes it through.
	attempt := 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var cmd *testexec.Cmd
		cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if network == "" {
			cmd = a.Command(cmdCtx, "dumpsys", "wifi", "tools", "reach", addr)
		} else {
			cmd = a.Command(cmdCtx, "dumpsys", "wifi", "tools", "reach", network, addr)
		}
		attempt++
		testing.ContextLogf(ctx, "Running `%s` (attempt #%d)", strings.Join(cmd.Args, " "), attempt)
		if o, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to execute 'reach' command, output: %s", string(o))
		} else if !strings.Contains(string(o), ": reachable") {
			return errors.Errorf("ping was unreachable, output: %s", string(o))
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "no response received in ARC")
	}

	return nil
}

// HideUnusedEthernet finds all Ethernet devices that's not being used and hide them
// from shill (thus patchpanel, and ARC) by Manager ClaimInterface API. This helps to
// workaround the current limitation of ARCVM that at most two ethernet devices can
// be supported. Returns a cleanup function to undo the changes.
func HideUnusedEthernet(ctx context.Context, manager *shill.Manager) (action.Action, error) {
	devices, err := manager.Devices(ctx)
	if err != nil {
		return nil, err
	}

	var toBeHidden []string
	for _, device := range devices {
		p, err := device.GetProperties(ctx)
		if err != nil {
			return nil, err
		}
		if tech, err := p.GetString(shillconst.DevicePropertyType); err != nil {
			return nil, err
		} else if tech != shillconst.TypeEthernet {
			continue
		}
		if linkUp, err := p.GetBool(shillconst.DevicePropertyEthernetLinkUp); err != nil {
			return nil, err
		} else if linkUp {
			// Do not hide up device to avoid breaking SSH to DUT
			continue
		}
		ifname, err := p.GetString(shillconst.DevicePropertyName)
		toBeHidden = append(toBeHidden, ifname)
	}

	for _, ifname := range toBeHidden {
		if err := manager.ClaimInterface(ctx, "tast", ifname); err != nil {
			return nil, errors.Wrapf(err, "failed to claim interface %s", ifname)
		}
		testing.ContextLogf(ctx, "Claimed interface %s from shill", ifname)
	}

	return func(ctx context.Context) error {
		for _, ifname := range toBeHidden {
			if err := manager.ReleaseInterface(ctx, "tast", ifname); err != nil {
				return errors.Wrapf(err, "failed to release interface %s", ifname)
			}
			testing.ContextLogf(ctx, "Released interface %s to shill", ifname)
		}
		return nil
	}, nil
}

// SaveNetworkDumpsys saves 'adb shell dumpsys' output of the ARC networking and wifi state to a
// log file.
func SaveNetworkDumpsys(ctx context.Context, a *arc.ARC, outDir string) error {
	dateString := time.Now().Format(time.RFC3339) // "2006-01-02T15:04:05Z07:00" format
	filename := "arc-network-dumpsys_" + dateString + ".txt"
	path := filepath.Join(outDir, filename)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "dumpsys", "wifi", "mojo", "networks", "arc-networks", "vpn", "proxy", "system", "arc-host-vpn")
	cmd.Stdout = file
	return cmd.Run(testexec.DumpLogOnError)
}

// saveARCSocketInfoDump saves 'adb shell ss' output of the ARC sockets to a log file
func saveARCSocketInfoDump(ctx context.Context, a *arc.ARC, outDir string) error {
	dateString := time.Now().Format(time.RFC3339) // "2006-01-02T15:04:05Z07:00" format
	filename := "arc-network-info_" + dateString + ".txt"
	path := filepath.Join(outDir, filename)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "ss", "-api")
	cmd.Stdout = file
	return cmd.Run(testexec.DumpLogOnError)
}

// CreateNetworkDumpsysErrorHandler creates an error handler to dump network info and ARC socket
// info on test failures. This should be used together with s.AttachErrorHandlers(). Example:
//
//	errorHandler := arc.CreateNetworkDumpsysErrorHandler(cleanupCtx, a)
//	s.AttachErrorHandlers(errorHandler, errorHandler)
func CreateNetworkDumpsysErrorHandler(ctx context.Context, a *arc.ARC) func(string) {
	return func(string) {
		outdir, ok := testing.ContextOutDir(ctx)
		if !ok {
			testing.ContextLog(ctx, "Failed to get context output directory")
		}
		if err := SaveNetworkDumpsys(ctx, a, outdir); err != nil {
			testing.ContextLog(ctx, "Failed to save ARC network dumpsys: ", err)
		}
		if err := saveARCSocketInfoDump(ctx, a, outdir); err != nil {
			testing.ContextLog(ctx, "Failed to save ARC network info dump: ", err)
		}
	}
}

func getPPNetworkDevice(ctx context.Context, hostIfname string) (*pp.NetworkDevice, error) {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create patchpanel client")
	}
	response, err := pc.GetDevices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get patchpanel devices")
	}
	for _, device := range response.Devices {
		if (device.GuestType == pp.NetworkDevice_ARCVM || device.GuestType == pp.NetworkDevice_ARC) && device.PhysIfname == hostIfname {
			return device, nil
		}
	}
	return nil, errors.Errorf("no ARC device matching %s is found", hostIfname)
}

// GetARCInterfaceName finds the interface name inside ARC given the host physical interface name.
func GetARCInterfaceName(ctx context.Context, hostIfname string) (string, error) {
	device, err := getPPNetworkDevice(ctx, hostIfname)
	if err != nil {
		return "", err
	}
	return device.GuestIfname, nil
}

// WaitForARCGetDNSProxyConfig waits for the connectivity manager in ARC gets
// the ipv4 and/or ipv6 dnsproxy address for the corresponding interface of
// hostIfname on the host side.
func WaitForARCGetDNSProxyConfig(ctx context.Context, a *arc.ARC, hostIfname string, ipv4, ipv6 bool, timeout time.Duration) error {
	// Get the interface and dnsproxy addrs from patchpanel.
	device, err := getPPNetworkDevice(ctx, hostIfname)
	if err != nil {
		return err
	}

	guestIfname := device.GuestIfname
	ipv4DNS := net.IP(device.DnsProxyIpv4Addr)
	ipv6DNS := net.IP(device.DnsProxyIpv6Addr)

	// dnsproxy addresses should always be set (no matter the actual IP
	// connectivity on the physical interface).
	if len(ipv4DNS) == 0 {
		return errors.New("got empty IPv4 DNS address")
	}
	if len(ipv6DNS) == 0 {
		return errors.New("got empty IPv6 DNS address")
	}

	var addrsForLog []string
	if ipv4 {
		addrsForLog = append(addrsForLog, ipv4DNS.String())
	}
	if ipv6 {
		addrsForLog = append(addrsForLog, ipv6DNS.String())
	}
	testing.ContextLogf(ctx, "Waiting for DNS %v to appear for interface %s in ARC", addrsForLog, guestIfname)

	// Poll the output of `dumpsys connectivity networks` to see if addrs are
	// applied.
	return testing.Poll(ctx, func(ctx context.Context) error {
		output, err := a.Command(ctx, "dumpsys", "connectivity", "networks").Output(testexec.DumpLogOnError)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to run `dumpsys connectivity networks`"))
		}

		// Checks that the DNS addresses are on the same line with the interface.
		// This is a very loose check, but since the addr for dnsproxy should not be
		// used elsewhere, this should be good enough. Assumption: "InterfaceName: "
		// won't appear on other lines than the one we want to check.
		for _, line := range strings.Split(string(output), "\n") {
			if !strings.Contains(line, "InterfaceName: "+guestIfname) {
				continue
			}
			if ipv4 && !strings.Contains(line, ipv4DNS.String()) {
				return errors.Errorf("IPv4 DNS `%s` is not in line `%s`", ipv4DNS, line)
			}
			if ipv6 && !strings.Contains(line, ipv6DNS.String()) {
				return errors.Errorf("IPv6 DNS `%s` is not in line `%s`", ipv6DNS, line)
			}
			return nil
		}
		return errors.Errorf("failed to find interface `%s` in output `%s`", guestIfname, string(output))
	}, &testing.PollOptions{Timeout: timeout})
}
