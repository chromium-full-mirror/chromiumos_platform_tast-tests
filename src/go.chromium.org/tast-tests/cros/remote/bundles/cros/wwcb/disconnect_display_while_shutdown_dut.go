// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisconnectDisplayWhileShutdownDUT,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Disconnect external display while shutdown DUT",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp", "newTestItem"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
		},
		Data: []string{"Capabilities.json", utils.VideoFile},
		Params: []testing.Param{
			{
				Name:      "full",
				ExtraAttr: []string{"pasit_full"},
			}},
	})
}

func DisconnectDisplayWhileShutdownDUT(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extDispID := s.RequiredVar("ExtDispID1")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to initialize servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	dockingID, hasDockingID := s.Var("DockingID")
	if hasDockingID {
		ipPowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
			s.Fatal("Failed to open IP power: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

		if _, ok := s.Var("newTestItem"); ok {
			if err := utils.VerifyDockingInterface(ctx, s.DUT(), dockingID, s.DataPath("Capabilities.json")); err != nil {
				s.Fatal("Failed to verify the docking station interface: ", err)
			}
		}

		if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
			s.Fatal("Failed to connect to the docking station: ", err)
		}
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	displayIDs, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	} else if len(displayIDs.DisplayIds) < 2 {
		s.Fatal("Failed to get display ID; it must be greater than or equal to 2")
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	if err := utils.OpenRGBImageOnDisplays(ctx, fs, appsSvc, uiautoSvc, displaySvc); err != nil {
		s.Fatal("Failed to open RGB image on each display: ", err)
	}
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60})

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	if err := utils.PairWebcamToDisplay(ctx, s, displayIDs.DisplayIds); err != nil {
		s.Fatal("Failed to pair webcam to display: ", err)
	}

	screenOn, err := utils.GetGamLightingValue(ctx, s, displayIDs.DisplayIds[0])
	if err != nil {
		s.Fatal("Failed to get DUT screen light while DUT is turned on: ", err)
	}

	if err := utils.ShutdownDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}
	defer utils.PowerOnDUT(cleanupCtx, pxy, dut)

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect the external display: ", err)
	}

	screenOff, err := utils.GetGamLightingValue(ctx, s, displayIDs.DisplayIds[0])
	if err != nil {
		s.Fatal("Failed to get DUT screen light while DUT is shutdown: ", err)
	}

	if screenOff >= screenOn {
		s.Fatalf("Expect DUT screen light in shutdown state is lower than in turned-on state; shutdown light value: %d, turned-on light value: %d", screenOff, screenOn)
	}
}
