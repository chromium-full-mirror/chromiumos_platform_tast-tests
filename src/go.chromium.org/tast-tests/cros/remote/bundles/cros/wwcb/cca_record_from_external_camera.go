// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

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
		Func:         CCARecordFromExternalCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Connect an external camera and use the CCA app to test the photo-taking and video-recording functionalities to ensure they are working properly",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
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
func CCARecordFromExternalCamera(ctx context.Context, s *testing.State) {
	/*
		1. Boot and loging to ChromeOS.
		2. Plug the USB webcam to the Chromebook. (turn on USB Test Fixture)
		3. Launch ""Camera"" app.
		4. Press the camera switch button.
		5. Take a photo.
		6. Check the photo looks good.
		7. Change the camera app to Video mode.
		8. Take a one minute video.
		9. Check the video looks good.
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
	defer fs.Remove(cleanupCtx, testImageFilepath)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	if _, err := utils.OpenGalleryOnDisplay(ctx, appsSvc, uiautoSvc, displaySvc, 1, testImageFilename); err != nil {
		s.Fatal("Failed to open file in Gallery on expected display: ", err)
	}
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60})
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
	builtinDevices, err := utils.USBCamerasFromV4L2Test(ctx, dut)
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

	extCamera, err := utils.ConnectExternalCamera(ctx, dut, extCameraID)
	if err != nil {
		s.Fatal("Failed to plug in external camera: ", err)
	}
	testing.ContextLogf(ctx, "Found external camera: %s", extCamera)

	// Launch Camera app.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(cleanupCtx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failed to wait for camera window to be stabled: ", err)
	}

	// Switch to external camera.
	if err := utils.SwitchCCADevice(ctx, dut, uiautoSvc, extCamera); err != nil {
		s.Fatal("Failed to switch CCA camera to external camera: ", err)
	}

	// Take a photo and verify the image color.
	photo, err := utils.TakeSinglePhoto(ctx, uiautoSvc, fs, utils.CameraPath)
	if err != nil {
		s.Fatal("Failed to take single photo: ", err)
	}

	photoPath, err := utils.CopyRemoteFile(ctx, fs, filepath.Join(utils.CameraPath, photo.Name()), s.OutDir())
	if err != nil {
		s.Error("Failed to copy remote file to local host dir: ", err)
	}

	f, err := os.Open(photoPath)
	if err != nil {
		s.Fatal("Failed to open file: ", err)
	}
	defer f.Close()

	image, _, err := image.Decode(f)
	if err != nil {
		s.Fatal("Failed to decode image: ", err)
	}

	if err := utils.ValidateImageColor(ctx, image, color.RGBA{255, 0, 0, 255}, 60); err != nil {
		s.Fatal("Failed to validate color of image captured from the external camera: ", err)
	}

	// Record a video and verify the frame color in the video.
	if err := utils.SwitchCCAMode(ctx, uiautoSvc, utils.VideoMode); err != nil {
		s.Fatal("Failed to switch video mode: ", err)
	}

	video, err := utils.RecordVideo(ctx, uiautoSvc, fs, 1*time.Second, utils.CameraPath)
	if err != nil {
		s.Fatal("Failed to record video: ", err)
	}

	videoPath, err := utils.CopyRemoteFile(ctx, fs, filepath.Join(utils.CameraPath, video.Name()), s.OutDir())
	if err != nil {
		s.Fatal("Failed to copy remote file to local host dir: ", err)
	}

	if err := utils.ValidateVideoColor(ctx, videoPath, s.OutDir()); err != nil {
		s.Fatal("Failed to validate color of video captured from the external camera: ", err)
	}
}
