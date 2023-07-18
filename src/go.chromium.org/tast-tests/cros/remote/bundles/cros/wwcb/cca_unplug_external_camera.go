// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"image/color"
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
		Func:         CCAUnplugExternalCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the CCA app can return to the front camera preview screen after removing the external camera",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "ExtCameraID", "wwcbIPPowerIp"},
		Data:         []string{utils.VideoFile},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.inputs.KeyboardService",
		},
	})
}
func CCAUnplugExternalCamera(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extCameraID := s.RequiredVar("ExtCameraID")
	var appTimeout int32
	appTimeout = 60

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

	fs := dutfs.NewClient(cl.Conn)

	testImageFile := filepath.Join(utils.MyFilesPath, utils.PictureFile)
	testImage := utils.GenerateImage(3840, 2160, color.RGBA{255, 0, 0, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, testImage, testImageFile); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(cleanupCtx, testImageFile)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: appTimeout}); err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: appTimeout})

	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	galleryWindow, err := utils.OpenMediaFileWithGallery(ctx, uiautoSvc, utils.PictureFile)
	if err != nil {
		s.Fatal("Failed to open media file on Files app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: appTimeout})

	if err := utils.ClickOnMaximizeButton(ctx, uiautoSvc, galleryWindow); err != nil {
		s.Fatal("Failed to click maximize button on the Gallery window: ", err)
	}

	if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: galleryWindow}); err != nil {
		s.Fatal("Failed to switch Gallery window to external display: ", err)
	}

	// Close files window.
	appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: appTimeout})

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: appTimeout}); err != nil {
		s.Fatal("Failed to launch camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: appTimeout})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failedt to wait camera app to be stable: ", err)
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			uiTreeResponse, err := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			if err != nil {
				s.Fatal("Failed to get UI Tree string: ", err)
			}
			s.Log(uiTreeResponse.UiTree)
		}
	}(cleanupCtx)

	if err := utils.ControlFixture(ctx, extCameraID, "on"); err != nil {
		s.Fatal("Failed to turn on fixture of external camera: ", err)
	}

	if err := utils.FindInfoOnCameraApp(ctx, uiautoSvc, utils.CCAPlugged); err != nil {
		s.Fatal("Failed to wait for Camera app to show the certain info: ", err)
	}

	if err := utils.SwitchCCACamera(ctx, dut, uiautoSvc); err != nil {
		s.Fatal("Failed to switch Camera app to external camera: ", err)
	}

	if err := utils.ControlFixture(ctx, extCameraID, "off"); err != nil {
		s.Fatal("Failed to turn on fixture of external camera: ", err)
	}

	photo, err := utils.TakeSinglePhoto(ctx, uiautoSvc, fs, utils.CameraPath)
	if err != nil {
		s.Fatal("Failed to take single photo: ", err)
	}

	imgPath, err := utils.CopyRemoteFile(ctx, fs, filepath.Join(utils.CameraPath, photo.Name()), s.OutDir())
	if err != nil {
		s.Error("Failed to copy remote file to local host dir: ", err)
	}

	if err := utils.ValidateImgColor(ctx, imgPath, "red"); err != nil {
		s.Fatal("Failed to validate color of image captured from the external camera: ", err)
	}
}
