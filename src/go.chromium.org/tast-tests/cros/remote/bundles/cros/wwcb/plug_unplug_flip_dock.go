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
	"google.golang.org/grpc"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlugUnplugFlipDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "In clamshell and tablet modes, verify peripherals connected via docking station while re-plug in and flip the dock station",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
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
		}, {
			Name:      "smoke",
			ExtraAttr: []string{"pasit_fast"},
			Val:       true,
		}},
	})
}

func PlugUnplugFlipDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")
	ethernetID := s.RequiredVar("EthernetID")
	usbDeviceIDs := strings.Split(s.RequiredVar("USBTypeAIDArray"), ",")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
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

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Connect non-power docking station.
	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to plug in the docking station: ", err)
	}

	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to power on the docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

	usbDevices, err := utils.ConnectPeripheralsViaDock(ctx, dut, extDispID, ethernetID, usbDeviceIDs)
	if err != nil {
		s.Fatal("Failed to plug in peripherals: ", err)
	}
	defer func(context.Context) {
		if s.HasError() {
			usbDevicesFailed, err := utils.GetUSBDevice(ctx, dut)
			if err != nil {
				s.Log("Failed to get the USB device when fail the test: ", err)
			}
			testing.ContextLog(ctx, strings.Join(usbDevicesFailed, "\n"))
		}
	}(cleanupCtx)

	for _, test := range []struct {
		ctrl string
		desc string
	}{
		{"on", "connect"},
		{"flip", "flip"},
	} {
		if err := utils.ControlFixture(ctx, dockingID, "off"); err != nil {
			s.Fatal("Failed to control fixture to disconnect the docking station: ", err)
		}

		// GoBigSleepLint: When disconnecting and reconnecting quickly,
		// prevent the docking or Chromebook from being temporarily cached and affecting the test results.
		testing.Sleep(ctx, 5*time.Second)

		if err := utils.ControlFixture(ctx, dockingID, test.ctrl); err != nil {
			s.Fatalf("Failed to control fixture to %s the docking station: %v", test.desc, err)
		}

		if err := utils.VerifyPeripheralsConnection(ctx, dut, true, usbDevices); err != nil {
			s.Fatal("Failed to verify connection of the peripherals: ", err)
		}
	}
}
