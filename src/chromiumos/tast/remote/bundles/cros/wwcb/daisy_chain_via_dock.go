// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
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
		Func:         DaisyChainViaDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Daisy chain two external display together via Dock, do video verification by camera connecting to the host",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "ExtDispID2", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.wwcb.DisplayService", "tast.cros.inputs.KeyboardService"},
		Data:         []string{utils.VideoFile},
	})
}

func DaisyChainViaDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID1 := s.RequiredVar("ExtDispID1")
	extDispID2 := s.RequiredVar("ExtDispID2")

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

	// Push file to remote.
	dut := s.DUT()
	remoteTXTPath, err := utils.PushFileToDUT(ctx, s, dut, utils.VideoFile, "/home/chronos/user/MyFiles/")
	if err != nil {
		s.Fatal("Failed to push file to DUT's MyFiles directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTXTPath).Output()

	// Open IP power to supply docking power.
	ipPowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
		s.Fatal("Failed to power on docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	extDispIDArray := []string{extDispID1, extDispID2}
	if err := utils.MappingWithDockFixture(ctx, s, extDispIDArray, dockingID); err != nil {
		s.Fatal("Failed to mapping display fixture to camera: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID1, "on"); err != nil {
		s.Fatal("Failed to connect to the first external display: ", err)
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	twoDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID2, "on"); err != nil {
		s.Fatal("Failed to connect to the second external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 3}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	threeDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	}

	// Rearrange input parameters if different with display sequence in DUT.
	if twoDisplays.DisplayIds[1] != threeDisplays.DisplayIds[1] {
		extDispID1, extDispID2 = extDispID2, extDispID1
	}

	// GoBigSleepLint: Wait for the monitor to turn on screen.
	testing.Sleep(ctx, 30*time.Second)

	filesWindowName := "Files - My files"
	galleryWindowName := fmt.Sprintf("Gallery - %s", utils.VideoFile)

	for _, test := range []struct {
		extDispIndex int
		extDispID    string
	}{
		{1, extDispID1},
		{2, extDispID2},
	} {
		testing.ContextLogf(ctx, "Play and verify the video on the external display %d with ID %s", test.extDispIndex, test.extDispID)

		if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
			s.Fatal("Failed to launch Filesapp: ", err)
		}
		defer utils.CloseWindow(ctx, keyboardSvc, uiautoSvc, filesWindowName)

		if err := utils.OpenMediaFileOnFilesapp(ctx, uiautoSvc, utils.VideoFile); err != nil {
			s.Fatal("Failed to open media file on Filesapp: ", err)
		}
		defer utils.CloseWindow(ctx, keyboardSvc, uiautoSvc, galleryWindowName)

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: int32(test.extDispIndex), WindowTitle: galleryWindowName}); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
			s.Fatal("Failed to switch Gallery window to the external display: ", err)
		}

		if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "fullscreen"}); err != nil {
			s.Fatal("Failed to type fullscreen: ", err)
		}

		if err := utils.ClickOnPlayButton(ctx, uiautoSvc); err != nil {
			s.Fatal("Failed to click on play button on the Gallery: ", err)
		}

		if err := utils.VerifyVideo(ctx, s, test.extDispID, 30); err != nil {
			s.Fatal("Failed to verify video on the external display: ", err)
		}
	}
}
