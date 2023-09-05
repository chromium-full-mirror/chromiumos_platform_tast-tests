// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
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
		Func:         DisconnectDisplayWhileSuspendDUT,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Disconnect the external display while DUT is in suspend mode",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.wwcb.DisplayService", "tast.cros.inputs.KeyboardService"},
		Data:         []string{utils.VideoFile},
	})
}
func DisconnectDisplayWhileSuspendDUT(ctx context.Context, s *testing.State) {
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
		s.Fatal("Failed to initialize the start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	extDispIDArray := []string{extDispID}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if dockingID, ok := s.Var("DockingID"); ok {
		ippowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
			s.Fatal("Failed to power on the docking station: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ippowerPorts)
		if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
			s.Fatal("Failed to connect to the docking station: ", err)
		}
	}
	fs := dutfs.NewClient(cl.Conn)
	if err := utils.MappingWebcam(ctx, s, fs, keyboardSvc, displaySvc, appsSvc, uiautoSvc, extDispIDArray); err != nil {
		s.Fatal("Failed to initialize the mapping webcams: ", err)
	}
	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	displayIDs, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	}

	fileName := "white.jpg"
	imageFile := filepath.Join(utils.MyFilesPath, fileName)
	image := utils.GenerateImage(3840, 2160, color.RGBA{255, 255, 255, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, image, imageFile); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(ctx, imageFile)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Filesapp: ", err)
	}

	// Open white.jpg on chromebook monitor.
	galleryWindowName := fmt.Sprintf("Gallery - %s", fileName)
	if err := utils.OpenMediaFileOnFilesapp(ctx, uiautoSvc, fileName); err != nil {
		s.Fatal("Failed to open media file on Filesapp: ", err)
	}
	defer utils.CloseWindow(ctx, keyboardSvc, uiautoSvc, galleryWindowName)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0, WindowTitle: galleryWindowName}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to switch Gallery window to the external display: ", err)
	}

	normalScreenLight, err := utils.GetGamLightingValue(ctx, s, displayIDs.DisplayIds[0])
	if err != nil {
		s.Fatal("Failed to get DUT screen light from camera after switch Gallery window to the external display: ", err)
	}

	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		s.Fatal("Failed to suspend DUT: ", err)
	}
	defer utils.PowerOnDUT(ctx, pxy, dut)

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		s.Fatal("Failed to disconnect to the external display after suspend: ", err)
	}

	sdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal(ctx, "Failed to wait for DUT to be unreachable: ", err)
	}

	suspendScreenLight, err := utils.GetGamLightingValue(ctx, s, displayIDs.DisplayIds[0])
	if err != nil {
		s.Fatal("Failed to get DUT screen light from camera after suspend and disconnect to the external display: ", err)
	}

	if suspendScreenLight >= normalScreenLight {
		s.Fatalf("Expect DUT screen light of suspend state is equal to or lower than in normal state; suspend light value: %d, normal light value: %d", suspendScreenLight, normalScreenLight)
	}
}
