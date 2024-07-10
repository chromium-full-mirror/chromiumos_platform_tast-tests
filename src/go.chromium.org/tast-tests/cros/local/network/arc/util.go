// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc provides ARC-related networking functionality.
package arc

import (
	"context"
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

const (
	// VethPrefix is a prefix for host veth interfaces name.
	VethPrefix = "veth_"
	// BridgePrefix is a prefix for host bridge interfaces name.
	BridgePrefix = "arc_"
	// VmtapPrefix is a prefix for host vmtap interfaces name.
	VmtapPrefix = "vmtap"

	// ARCBR0 refers to a host network bridge interface named arcbr0.
	ARCBR0 = "arcbr0"
	// ARC0 refers to an ARC network interface named arc0.
	ARC0 = "arc0"
	// ARC1 refers to an ARC network interface named arc1 which is created for ARCVM.
	// On dual-boards, ARC++ might have this interface as a result of switching ARCVM -> ARC++.
	// ARC++ should not care about this interface.
	ARC1 = "arc1"
	// Loopback refers to loopback interface named lo.
	Loopback = "lo"

	// Android interface prefixes
	clatPrefix = "v4-"
	vpnPrefix  = "tun"
)

// NetworkInterfaceNames filters Android interfaces and returns ARC related network interfaces.
func NetworkInterfaceNames(ctx context.Context) ([]string, error) {
	out, err := arc.BootstrapCommand(ctx, "/system/bin/ls", "/sys/class/net/").Output()
	if err != nil {
		return nil, err
	}

	// Filter out non-arc android net interfaces
	var ifnames []string
	for _, ifname := range strings.Fields(string(out)) {
		if !strings.HasPrefix(ifname, clatPrefix) &&
			!strings.HasPrefix(ifname, vpnPrefix) &&
			ifname != Loopback {
			ifnames = append(ifnames, ifname)
		}
	}

	return ifnames, nil
}

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
	// TODO(cassiewang): We observed in the local manual tests that sometimes this
	// command gave: "*** SERVICE 'wifi' DUMP TIMEOUT (10000ms) EXPIRED ***". Need
	// to check if this also happens on the lab machines, so use a relatively
	// longer timeout here.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var cmd *testexec.Cmd
		if network == "" {
			cmd = a.Command(ctx, "dumpsys", "wifi", "tools", "reach", addr)
		} else {
			cmd = a.Command(ctx, "dumpsys", "wifi", "tools", "reach", network, addr)
		}
		if o, err := cmd.Output(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to execute 'reach' commmand, output: %s", string(o))
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

// GetARCInterfaceName finds the interface name inside ARC given the host physical interface name.
func GetARCInterfaceName(ctx context.Context, hostIfname string) (string, error) {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create patchpanel client")
	}
	response, err := pc.GetDevices(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get patchpanel devices")
	}
	for _, device := range response.Devices {
		if (device.GuestType == pp.NetworkDevice_ARCVM || device.GuestType == pp.NetworkDevice_ARC) && device.PhysIfname == hostIfname {
			return device.GuestIfname, nil
		}
	}
	return "", errors.Errorf("no ARC device matching %s is found", hostIfname)
}
