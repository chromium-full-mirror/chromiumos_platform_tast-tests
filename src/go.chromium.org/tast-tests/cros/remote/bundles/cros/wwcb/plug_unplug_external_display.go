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
		Func:         PlugUnplugExternalDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Plug in the external display then play video to check the external display is functional by the camera connecting to the host, then unplug the external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "newTestItem"},
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

func PlugUnplugExternalDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extDispID := s.RequiredVar("ExtDispID1")

	// Connect to the gRPC server on the DUT.
	dut := s.DUT()
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize the start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Connect the external display.
	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if dockingID, ok := s.Var("DockingID"); ok {
		ippowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
			s.Fatal("Failed to power on the docking station: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ippowerPorts)

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

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	if err := utils.PairWebcamToDisplay(ctx, s, displayIDs.DisplayIds); err != nil {
		s.Fatal("Failed to pair webcam to display: ", err)
	}

	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Gallery app: ", err)
	}

	// Open the video on the external display and verify the external display performace.
	remoteAudioPath, err := utils.PushFileToDUT(ctx, s, dut, utils.VideoFile, utils.MyFilesPath)
	if err != nil {
		s.Fatal("Failed to initialize the push file to DUT's MyFiles directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteAudioPath).Output()

	galleryTitle, err := utils.OpenGalleryOnDisplay(ctx, appsSvc, uiautoSvc, displaySvc, 1, utils.VideoFile)
	if err != nil {
		s.Fatal("Failed to open video on external display: ", err)
	}
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60})

	if _, err := displaySvc.EnsureSetWindowState(ctx, &wwcb.QueryRequest{WindowTitle: galleryTitle, WindowState: wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED}); err != nil {
		s.Fatalf("Failed to set the window state of the %s as %s: %v", galleryTitle, wwcb.WindowStateType_WINDOW_STATE_MAXIMIZED.String(), err)
	}

	if err := utils.ClickOnPlayButton(ctx, uiautoSvc); err != nil {
		s.Fatal("Failed to click on play button on the Gallery: ", err)
	}

	if err := utils.VerifyVideo(ctx, s, displayIDs.DisplayIds[1], 30); err != nil {
		s.Fatal("Failed to verify video on the external display: ", err)
	}

	// Unplug the external display and verify video switched back to primary display.
	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect the external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 1}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryTitle, DisplayIndex: 0}); err != nil {
		s.Fatal("Failed to verify video window switched back to the DUT: ", err)
	}
}
