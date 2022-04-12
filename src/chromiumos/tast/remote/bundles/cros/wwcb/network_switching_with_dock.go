// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook
package wwcb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/testing"
)

const (
	servoEth   = "eth0"
	dockingEth = "eth1"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkSwitchingWithDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test wired network when connecting/disconnecting over a Dock",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"DockingID", "ExtDispID1", "EthernetID", "wwcbIPPowerIp"},
	})
}

func NetworkSwitchingWithDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")
	ethernetID := s.RequiredVar("EthernetID")

	// Open IP power to supply docking power.
	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to open IP power: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect the external display to the Dock: ", err)
	}
	if err := utils.ControlFixture(ctx, ethernetID, "on"); err != nil {
		s.Fatal("Failed to connect the Ethernet to the Dock: ", err)
	}
	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect the docking station: ", err)
	}

	// Find Dock Ethernet on DUT.
	if err := findInterface(ctx, s.DUT(), dockingEth); err != nil {
		s.Fatal("Failed to find dock Ethernet interface: ", err)
	}

	server := "www.google.com"
	if err := pingNetwork(ctx, s.DUT(), dockingEth, server); err != nil {
		s.Fatal("Failed to check Dock Ethernet is enabled: ", err)
	}

	// Verify Dock Ethernet is closed in negative situation.
	if err := utils.ControlFixture(ctx, ethernetID, "off"); err != nil {
		s.Fatal("Failed to disconnect Ethernet from Dock: ", err)
	}
	if err := pingNetwork(ctx, s.DUT(), dockingEth, server); err == nil {
		s.Fatal("Expect the Ethernet interface in the Dock is disabled; however it is still available")
	}
}

// findInterface finds the certain interface name from ifconfig.
func findInterface(ctx context.Context, dut *dut.DUT, ifName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprint(`ifconfig -s`)
		out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return errors.Wrap(err, "failed to find interfaces")
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			elements := strings.Split(line, " ")
			if elements[0] == ifName {
				return nil
			}
		}
		return errors.Errorf("Unable to find the %s interface", ifName)
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Microsecond})
}

// pingNetwork verifies whether the network interface is available or not.
func pingNetwork(ctx context.Context, dut *dut.DUT, ifName, target string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		return dut.Conn().CommandContext(ctx, "ping", "-I", ifName, "-c", "3", target).Run()
	}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: 200 * time.Microsecond})
}
