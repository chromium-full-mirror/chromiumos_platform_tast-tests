// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"google.golang.org/grpc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendResumeDockPlugUnplug,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "In clamshell and tablet modes, verifies the connection of the peripherals after plug and unplug the docking station during DUT sleep",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "EthernetID", "USBTypeAIDArray", "wwcbIPPowerIp"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}},
	})
}

func SuspendResumeDockPlugUnplug(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Cutting off the servo power supply.
	if err := pxy.Servo().SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer pxy.Servo().SetPDRole(cleanupCtx, servo.PDRoleSrc)

	// Enable tablet mode.
	if s.Param().(bool) {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")
	ethernetID := s.RequiredVar("EthernetID")
	USBDeviceIDs := strings.Split(s.RequiredVar("USBTypeAIDArray"), ",")

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Open IP power to supply docking power.
	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to power on the docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to plug in the docking station: ", err)
	}

	USBDevices, err := utils.ConnectPeripheralsViaDock(ctx, dut, extDispID, ethernetID, USBDeviceIDs)
	if err != nil {
		s.Fatal("Failed to connect the peripherals via Dock: ", err)
	}

	if err := suspendResume(ctx, dut, pxy, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after suspend/resume DUT: ", err)
	}

	if err := suspendUnplugResumePlug(ctx, dut, pxy, dockingID, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after suspend DUT, unplug dock, resume DUT, plug dock: ", err)
	}

	if err := unplugSuspendPlugResume(ctx, dut, pxy, dockingID, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after unplug dock, suspend DUT, plug dock, resume DUT: ", err)
	}
}

// suspendResume suspends DUT for 15s and verifies the connection of peripherals.
func suspendResume(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy, USBDevices []string) error {
	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		return errors.Wrap(err, "perform powerdbus suspend")
	}

	// GoBigSleepLint: Suspend more than 10s as testing requirement.
	testing.Sleep(ctx, 15*time.Second)

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "power on DUT")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "verify peripherals connection")
	}
	return nil
}

// suspendUnplugResumePlug suspends DUT, unplugs the dock, resumes DUT, plugs in the dock then verifies the connection of peripherals.
func suspendUnplugResumePlug(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy, dockingID string, USBDevices []string) error {
	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		return errors.Wrap(err, "perform powerdbus suspend")
	}

	if err := utils.ControlFixture(ctx, dockingID, "off"); err != nil {
		return errors.Wrap(err, "unplug the docking station")
	}

	// Unplugging the dock may cause DUT wake up, and screen still remains dark, wait DUT into suspend.
	sCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sCtx); err != nil {
		return errors.Wrap(err, "wait DUT to be unreachable")
	}

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "power on DUT")
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		return errors.Wrap(err, "plug in the docking station")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "verify peripherals connection")
	}
	return nil
}

// unplugSuspendPlugResume unplugs the dock, suspends the DUT, plugs in the dock, resumes the DUT then verifies the connection of peripherals.
func unplugSuspendPlugResume(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy, dockingID string, USBDevices []string) error {
	if err := utils.ControlFixture(ctx, dockingID, "off"); err != nil {
		return errors.Wrap(err, "unplug the docking station")
	}

	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		return errors.Wrap(err, "perform powerdbus suspend")
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		return errors.Wrap(err, "plug in the docking station")
	}

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "power on DUT")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "verify peripherals connection")
	}
	return nil
}
