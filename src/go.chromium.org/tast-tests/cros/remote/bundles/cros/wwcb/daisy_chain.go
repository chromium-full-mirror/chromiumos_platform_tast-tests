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

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
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
		Func:         DaisyChain,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Daisy chain two external display together via Dock, do video verification by camera connecting to the host",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_display"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "newTestItem"},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.ui.ChromeUIService",
		},
		Data:    []string{"Capabilities.json", utils.VideoFile},
		Fixture: "wwcb.display",
	})
}

func DaisyChain(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Push file to remote.
	dut := s.DUT()
	remoteTXTPath, err := utils.PushFileToDUT(ctx, s, dut, utils.VideoFile, utils.MyFilesPath)
	if err != nil {
		s.Fatal("Failed to initialize the push file to DUT's MyFiles directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTXTPath).Output()

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	tf := s.FixtValue().(*topology.TestFixture)
	// Find a display whose connection path passes through another display.
	extDispID2, extDispID1, err := tf.Helper.DeviceByTypeVia(ctx, topology.DeviceTypeMonitor, topology.DeviceTypeMonitor)
	if err != nil {
		s.Fatal("Failed to activate monitor: ", err)
	}

	// Connect the first display and verify.
	if err := tf.Helper.ActivateDeviceByID(ctx, extDispID1); err != nil {
		s.Fatal("Failed to deactivate monitor: ", err)
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Connect the second display and verify.
	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count after connect first external display: ", err)
	}

	twoDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID after connect first external display: ", err)
	} else if len(twoDisplays.DisplayIds) < 2 {
		s.Fatal("Failed to get display ID;it must be greater than or equal to 2 after connect second external display")
	}

	if err := tf.Helper.ActivateDeviceByID(ctx, extDispID2); err != nil {
		s.Fatal("Failed to connect to the second external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 3}); err != nil {
		s.Fatal("Failed to verify display count after connect second external display: ", err)
	}

	threeDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID after connect second external display: ", err)
	} else if len(threeDisplays.DisplayIds) < 3 {
		s.Fatal("Failed to get display ID;it must be greater than or equal to 3 after connect second external display")
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

	if err := utils.PairWebcamToDisplay(ctx, s, threeDisplays.DisplayIds); err != nil {
		s.Fatal("Failed to pair webcam to display: ", err)
	}

	for _, test := range []struct {
		extDispIndex int
		extDispID    string
	}{
		{1, threeDisplays.DisplayIds[1]},
		{2, threeDisplays.DisplayIds[2]},
	} {
		testing.ContextLogf(ctx, "Play and verify the video on the external display %d with ID %s", test.extDispIndex, test.extDispID)

		windowTitle, err := utils.OpenGalleryOnDisplay(ctx, appsSvc, uiautoSvc, displaySvc, test.extDispIndex, utils.VideoFile)
		if err != nil {
			s.Fatal("Failed to open gallery on display: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: int32(test.extDispIndex), WindowTitle: windowTitle}); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
			s.Fatal("Failed to switch Gallery window to the external display: ", err)
		}

		if err := utils.ClickFullScreenButton(ctx, uiautoSvc); err != nil {
			s.Fatal("Failed to click on fullscreen on the Gallery: ", err)
		}

		if err := utils.ClickOnPlayButton(ctx, uiautoSvc); err != nil {
			s.Fatal("Failed to click on play button on the Gallery: ", err)
		}

		if err := utils.VerifyVideo(ctx, s, test.extDispID, 30); err != nil {
			s.Fatal("Failed to verify video on the external display: ", err)
		}

		if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
			s.Fatal("Failed to close Gallery: ", err)
		}
	}
}
