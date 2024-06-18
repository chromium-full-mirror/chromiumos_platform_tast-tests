// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

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
		Func:         CCASwitchToExternalCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launch camera app and check external webcam could be detected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_camera"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"ExtCameraID"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.ui.ChromeUIService",
		},
	})
}

func CCASwitchToExternalCamera(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extCameraID := s.RequiredVar("ExtCameraID")

	// Connect to the gRPC server on the DUT.
	dut := s.DUT()
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	s.AttachErrorHandlers(
		func(errMsg string) {
			utils.DumpUITreeWithScreenshotToFile(cleanupCtx, cl.Conn, "ui_dump_error")
		},
		func(errMsg string) {
			utils.DumpUITreeWithScreenshotToFile(cleanupCtx, cl.Conn, "ui_dump_fatal")
		},
	)

	// Because the hardware environment is designed so that the Chromebook’s front camera faces the external monitor.
	// Open the red image on the external display and check the CCA app preview.
	testImageFilename := "test_image_red_color.jpg"
	testImageFilepath := filepath.Join(utils.MyFilesPath, testImageFilename)
	testImage := utils.GenerateImage(3840, 2160, color.RGBA{255, 0, 0, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, testImage, testImageFilepath); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(ctx, testImageFilepath)

	if _, err := utils.OpenGalleryOnDisplay(ctx, appsSvc, uiautoSvc, displaySvc, 1, testImageFilename); err != nil {
		s.Fatal("Failed to open file in Gallery on expected display: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60})
	defer func(ctx context.Context) {
		if s.HasError() {
			uiTreeResponse, err := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			if err != nil {
				s.Log("Unable to get UI tree: ", err)
			}

			if err := os.WriteFile(filepath.Join(s.OutDir(), "ui_tree.txt"), []byte(uiTreeResponse.UiTree), 0644); err != nil {
				s.Log("Unable to save UI tree on the host: ", err)
			}
		}
	}(cleanupCtx)

	// Check USB webcam can be detect properly (lsusb, dmesg, etc...).
	builtinDevices, err := utils.DevicesFromV4L2(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get built-in devices from V4L2: ", err)
	}
	if len(builtinDevices) == 0 {
		s.Fatal("Expect to get at least one built-in device, but get nothing")
	}
	testing.ContextLog(ctx, "Found built-in camera: ", builtinDevices)

	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	extCamera, err := utils.ConnectExternalCamera(ctx, dut, extCameraID)
	if err != nil {
		s.Fatal("Failed to plug in external camera: ", err)
	}
	testing.ContextLogf(ctx, "Found external camera: %s", extCamera)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failed to wait for camera window to be stabled: ", err)
	}

	if err := utils.SwitchCCADevice(ctx, dut, uiautoSvc, extCamera); err != nil {
		s.Fatal("Failed to switch CCA camera to external camera: ", err)
	}
	testing.ContextLog(ctx, "Verify the camera app's preview is red")

	ccaPreviewImg, err := utils.CropCCAPreview(ctx, uiautoSvc, s.OutDir())
	if err != nil {
		s.Fatal("Failed to capture CCA preview: ", err)
	}

	if err := utils.ValidateImageColor(ctx, ccaPreviewImg, color.RGBA{240, 80, 80, 255}, 66); err != nil {
		s.Fatal("Failed to validate image color: ", err)
	}
}
