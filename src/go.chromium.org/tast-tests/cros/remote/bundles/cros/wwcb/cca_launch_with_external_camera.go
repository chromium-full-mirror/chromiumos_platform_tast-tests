// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"bytes"
	"context"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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
		Func:         CCALaunchWithExternalCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launch cca app with the external camera connected and check the preview of app is from the front camera",
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
func CCALaunchWithExternalCamera(ctx context.Context, s *testing.State) {
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

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Check USB webcam can be detect properly (lsusb, dmesg, etc...).
	builtin, err := utils.DevicesFromV4L2(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get list of v4l devices: ", err)
	}

	if err := utils.ControlFixture(ctx, extCameraID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the external camera: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		afterPlugin, err := utils.DevicesFromV4L2(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get list of v4l devices: ", err)
		}
		if len(builtin) == len(afterPlugin) {
			s.Fatalf("Expect the number of v4l devices would change, but it remains the same, got: %q", builtin)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to detect the external camera: ", err)
	}

	// Since the hardware environment is designed as the front camera of Chromebook heading to the external display.
	// Launch the Filesapp and open the testing image on the external display.
	// Check the camera app preview from the front camera of Chromebook.
	testImageFilename := "test_image_red_color.jpg"
	testImageFile := filepath.Join(utils.MyFilesPath, testImageFilename)
	testImage := utils.GenerateImage(3840, 2160, color.RGBA{255, 0, 0, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, testImage, testImageFile); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(ctx, testImageFile)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	galleryWindow, err := utils.OpenMediaFileWithGallery(ctx, uiautoSvc, testImageFilename)
	if err != nil {
		s.Fatal("Failed to open media file with Gallery app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60})

	if err := utils.ClickOnMaximizeButton(ctx, uiautoSvc, galleryWindow); err != nil {
		s.Fatal("Failed to click the maximize button on the Gallery window: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
			return err
		}

		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: galleryWindow}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to switch Gallery app to the external display: ", err)
	}

	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Filesapp: ", err)
	}

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failed to wait for camera window to be stabled: ", err)
	}

	// Check active camera is the front camera of Chromebook.
	var isCCAUseBuiltinCamera bool = false
	for _, d := range builtin {
		if strings.Contains(d, "/dev/video") {
			processes, _ := utils.ListProcessInfo(ctx, dut, d)
			if utils.Contains(processes, `arc-camera`) {
				isCCAUseBuiltinCamera = true
				s.Logf("CCA app is using the %s from the built-in camera", d)
				break
			}
		}
	}
	if !isCCAUseBuiltinCamera {
		s.Fatal("Expect CCA app is using built-in camera, but is not")
	}

	cameraScreenshot, err := utils.TakeWindowScreenshot(ctx, uiautoSvc, fs, utils.CameraWindowFinder)
	if err != nil {
		s.Fatal("Failed to take a window screenshot: ", err)
	}

	screenshot := filepath.Join(utils.DownloadsPath, cameraScreenshot.Name())
	imgBytes, err := fs.ReadFile(ctx, screenshot)
	if err != nil {
		s.Fatal("Failed to read files on the DUT: ", err)
	}

	img, err := png.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		s.Fatal("Failed to decode PNG file: ", err)
	}

	screenshotFile := filepath.Join(s.OutDir(), "camera_screenshot.jpg")
	file, err := os.Create(screenshotFile)
	if err != nil {
		s.Fatal("Failed to create file: ", err)
	}
	defer file.Close()

	// Encode to JPEG format to validate the color of the picture.
	if err := jpeg.Encode(file, img, nil); err != nil {
		s.Fatal("Failed to encode JPEG file: ", err)
	}

	if err := utils.ValidateImgColor(ctx, screenshotFile, "red"); err != nil {
		s.Fatal("Failed to validate image color: ", err)
	}
}
