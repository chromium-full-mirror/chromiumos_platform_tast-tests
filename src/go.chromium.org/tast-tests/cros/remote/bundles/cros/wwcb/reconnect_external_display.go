// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReconnectExternalDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check window position while connect/disconnect the external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp", "newTestItem"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.wwcb.DisplayService", "tast.cros.inputs.KeyboardService"},
		Data:         []string{"Capabilities.json"},
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func ReconnectExternalDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extDispID := s.RequiredVar("ExtDispID1")

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

	// Connect the external display & Dock.
	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect the external display: ", err)
	}

	if dockingID, ok := s.Var("DockingID"); ok {
		ipPowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
			s.Fatal("Failed to power on the docking station: ", err)
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
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	// Open any app for testing purpose.
	filesWindowName := "Files - My files"
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Filesapp: ", err)
	}
	defer utils.CloseWindow(cleanupCtx, keyboardSvc, uiautoSvc, filesWindowName)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: filesWindowName}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to switch Filesapp window to the external display: ", err)
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowName, DisplayIndex: 1}); err != nil {
		s.Fatal("Failed to verify Filesapp window show on the external display: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect the external display: ", err)
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowName, DisplayIndex: 0}); err != nil {
		s.Fatal("Failed to verify Filesapp window show on the DUT: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect the external display: ", err)
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowName, DisplayIndex: 1}); err != nil {
		s.Fatal("Failed to verify Filesapp window show on the external display after reconnect the external display: ", err)
	}
}
