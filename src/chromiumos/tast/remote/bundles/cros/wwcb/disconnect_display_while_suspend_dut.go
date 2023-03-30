// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/rpc"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wwcb"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisconnectDisplayWhileSuspendDUT,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Disconnect the external display while DUT is in suspend mode",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.wwcb.DisplayService"},
	})
}
func DisconnectDisplayWhileSuspendDUT(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

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

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)

	// Open IP power to supply docking power.
	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to power on docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	extDispIDArray := []string{extDispID}
	if err := utils.MappingWithDockFixture(ctx, s, extDispIDArray, dockingID); err != nil {
		s.Fatal("Failed to do mapping display fixture to camera: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	normalScreenLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get DUT screen light from camera: ", err)
	}

	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		s.Fatal("Failed to suspend DUT: ", err)
	}
	defer utils.PowerOnDUT(ctx, pxy, dut)

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect the external display: ", err)
	}

	suspendScreenLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get DUT screen light from camera: ", err)
	}

	if suspendScreenLight >= normalScreenLight {
		s.Fatalf("Expect DUT screen light of suspend state is equal to or lower than in normal state; suspend light value: %d, normal light value: %d", suspendScreenLight, normalScreenLight)
	}
}
