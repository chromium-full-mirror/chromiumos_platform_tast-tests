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
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
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
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.ChromeUIService"},
		Vars:         []string{"servo", "newTestItem"},
		TestBedDeps:  []string{tbdep.ServoState("WORKING")},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcb.dock",
		Timeout:      utils.TestingTimeout,
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}, {
			Name:      "fast",
			ExtraAttr: []string{"pasit_fast"},
			Val:       true,
		}},
	})
}

func PlugUnplugFlipDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Cutting off the servo power supply.
	if err := utils.DisableServoPower(ctx, dut, pxy.Servo()); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer utils.EnableServoPower(ctx, dut, pxy.Servo())

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

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	tf := s.FixtValue().(*topology.TestFixture)
	dockingID, usbDevices, err := tf.ConnectPeripheralsViaDock(ctx, dut)
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

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}

	for _, test := range []struct {
		ctrl string
		desc string
	}{
		{"on", "connect"},
		{"flip", "flip"},
	} {
		if err := tf.Helper.DeactivateDeviceByID(ctx, dockingID); err != nil {
			s.Fatal("Failed to control fixture to disconnect the docking station: ", err)
		}

		// GoBigSleepLint: When disconnecting and reconnecting quickly,
		// prevent the docking or Chromebook from being temporarily cached and affecting the test results.
		testing.Sleep(ctx, 5*time.Second)

		if test.ctrl == "on" {
			if err := tf.Helper.ActivateDeviceByID(ctx, dockingID); err != nil {
				s.Fatalf("Failed to control fixture to %s the docking station: %v", test.desc, err)
			}
		} else if test.ctrl == "flip" {
			if err := tf.Helper.FlipDeviceByID(ctx, dockingID); err != nil {
				s.Fatalf("Failed to control fixture to %s the docking station: %v", test.desc, err)
			}
		}

		if err := utils.VerifyPeripheralsConnection(ctx, dut, true, usbDevices); err != nil {
			s.Fatal("Failed to verify connection of the peripherals: ", err)
		}

		if _, ok := s.Var("newTestItem"); ok {
			if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
				s.Fatal("Failed to verify the docking station interface: ", err)
			}

			if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
				s.Fatal("Failed to verify the usb devices speed: ", err)
			}
		}
	}
}
