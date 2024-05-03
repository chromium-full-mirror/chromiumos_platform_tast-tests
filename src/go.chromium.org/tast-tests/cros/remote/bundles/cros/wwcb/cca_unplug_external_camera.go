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

	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUnplugExternalCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the CCA app can return to the front camera preview screen after removing the external camera",
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
		},
	})
}

// TODO: Since this function is verify big, need to refactor into more functions and utils.
func CCAUnplugExternalCamera(ctx context.Context, s *testing.State) {
	/*
		1. Boot and loging to ChromeOS.
		2. Plug the USB webcam to the Chromebook. (turn on USB Test Fixture)
		3. Launch "Camera" app.
		4. Check active camera is the front camera of Chromebook.
		5. Press the camera switch button.
		6. Unplug the external camera. (turn off USB Switch Fixture)
		7. Check active camera is the front camera of Chromebook.
		8. Check the camera app preview switch to front camera of Chromebook.
	*/
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

	// Start Chrome on the DUT.
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

	// Open the red image on the external display.
	testImageFilename := "test_image_red_color.jpg"
	testImageFilepath := filepath.Join(utils.MyFilesPath, testImageFilename)
	testImage := utils.GenerateImage(3840, 2160, color.RGBA{255, 0, 0, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, testImage, testImageFilepath); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(ctx, testImageFilepath)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

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

	// Retrieve the built-in camera.
	builtinDevices, err := utils.DevicesFromV4L2(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get built-in devices from V4L2: ", err)
	}
	if len(builtinDevices) == 0 {
		s.Fatal("Expect to get at least one built-in device, but get nothing")
	}
	testing.ContextLog(ctx, "Found built-in camera: ", builtinDevices)

	// Connect the external camera via controlling the fixture.
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

	// Launch the camera app.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failed to wait for camera window to be stabled: ", err)
	}

	// Switch to the external camera.
	if err := utils.SwitchCCADevice(ctx, dut, uiautoSvc, extCamera); err != nil {
		s.Fatal("Failed to switch CCA camera to external camera: ", err)
	}

	// Disconnect the external camera.
	if err := utils.ControlFixture(ctx, extCameraID, "off"); err != nil {
		s.Fatal("Failed to turn on fixture of external camera: ", err)
	}

	// Check CCA app is using built-in camera.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		currentDevice, err := utils.CCAUseDevice(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve CCA app is using which camera")
		}

		if !utils.Contains(builtinDevices, currentDevice) {
			return errors.Errorf("Unexpect CCA use device; got: %s, want: %v", currentDevice, builtinDevices)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to check the CCA app is using built-in devices: ", err)
	}

	// Check CCA preview is ok.
	ccaPreviewImg, err := utils.CropCCAPreview(ctx, uiautoSvc, s.OutDir())
	if err != nil {
		s.Fatal("Failed to capture CCA preview: ", err)
	}

	clr, ratio := colorcmp.DominantColor(ccaPreviewImg)
	testing.ContextLogf(ctx, "CCA preview dominant color: %v, ratio: %f", clr, ratio)
	if ratio > 95 {
		s.Fatal("Expect CCA preview to show the real world; it shouldn't have just one color with more than 95% proportion")
	}
}
