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

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/rpc"
	pb "chromiumos/tast/services/cros/apps"
	inputspb "chromiumos/tast/services/cros/inputs"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wwcb"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlugUnplugExtDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Connect / disconnect external display via Dock then play video to check external display is functional by camera connecting to the host",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.wwcb.DisplayService", "tast.cros.inputs.KeyboardService"},
		Data:         []string{utils.VideoFile},
	})
}

func PlugUnplugExtDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
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

	// Push file to remote.
	dut := s.DUT()
	remoteAudioPath, err := utils.PushFileToDUT(ctx, s, dut, utils.VideoFile, utils.MyFilesPath)
	if err != nil {
		s.Fatal("Failed to push file to DUT's MyFiles directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteAudioPath).Output()

	// Open IP power to supply docking power.
	ipPowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
		s.Fatal("Failed to power on the docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize the webcam: ", err)
	}

	extDispIDArray := []string{extDispID}
	if err := utils.MappingWithDockFixture(ctx, s, extDispIDArray, dockingID); err != nil {
		s.Fatal("Failed to mapping display fixture to camera: ", err)
	}

	// Connect the external display & Dock.
	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	// Open any app for testing purpose. Choose the built-in Filesapp.
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
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to switch Filesapp window to the external display: ", err)
	}

	if err := utils.OpenMediaFileOnFilesapp(ctx, uiautoSvc, utils.VideoFile); err != nil {
		s.Fatal("Failed to open a video file on the Filesapp: ", err)
	}

	// Switch window to the external display.
	galleryWindowName := fmt.Sprintf("Gallery - %s", utils.VideoFile)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: galleryWindowName}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to switch Gallery window to the external display: ", err)
	}
	defer utils.CloseWindow(cleanupCtx, keyboardSvc, uiautoSvc, galleryWindowName)

	if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "fullscreen"}); err != nil {
		s.Fatal("Failed to click fullscreen key: ", err)
	}

	if err := utils.ClickOnPlayButton(ctx, uiautoSvc); err != nil {
		s.Fatal("Failed to click on play button on the Gallery: ", err)
	}

	if err := utils.VerifyVideo(ctx, s, extDispID, 30); err != nil {
		s.Fatal("Failed to verify video on the external display: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect the external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 1}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowName, DisplayIndex: 0}); err != nil {
		s.Fatal("Failed to verify Filesapp window switched back to the DUT: ", err)
	}
}
