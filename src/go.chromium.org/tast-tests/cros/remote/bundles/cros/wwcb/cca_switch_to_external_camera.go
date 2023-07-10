// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
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
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"ExtCameraID"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.inputs.KeyboardService",
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

	testing.ContextLog(ctx, "Starting Chrome on the DUT")

	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})
	testing.ContextLog(ctx, "Initializing fixtures")
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	fileName := "red.jpg"
	imageFile := filepath.Join(utils.MyFilesPath, fileName)
	image := utils.GenerateImage(3840, 2160, color.RGBA{255, 0, 0, 255})
	if err := utils.WriteImageOnDUT(ctx, fs, image, imageFile); err != nil {
		s.Fatal("Failed to write test image on DUT: ", err)
	}
	defer fs.Remove(ctx, imageFile)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Filesapp: ", err)
	}

	// Open red.jpg on external monitor.
	galleryWindowName := fmt.Sprintf("Gallery - %s", fileName)
	if err := utils.OpenMediaFileOnFilesapp(ctx, uiautoSvc, fileName); err != nil {
		s.Fatal("Failed to open media file on Filesapp: ", err)
	}
	defer utils.CloseWindow(ctx, keyboardSvc, uiautoSvc, galleryWindowName)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: galleryWindowName}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to switch Gallery window to the external display: ", err)
	}

	// Click the 'maximize' button to maximize the windows on the screen.
	// Click the button twice to ensure all gallery windows maximize.
	// Sometimes click one will not maximize the gallery window on the external display.
	// Maximize gallery window to let webcam more precise to verify.
	if err := utils.ClickOnMaximizeButton(ctx, uiautoSvc, galleryWindowName); err != nil {
		testing.ContextLogf(ctx, "Failed to click on maximize button: %s", err)
	}

	if err := utils.ClickOnMaximizeButton(ctx, uiautoSvc, galleryWindowName); err != nil {
		testing.ContextLogf(ctx, "Failed to click on maximize button: %s", err)
	}

	testing.ContextLog(ctx, "Launch Camera app")

	if err := launchCameraApp(ctx, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})
	defer func() {
		if s.HasError() {
			uiTreeResponse, err := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			if err != nil {
				testing.ContextLog(ctx, "Failed to get UI Tree string")
			}
			s.Log(uiTreeResponse.UiTree)
		}
	}()

	testing.ContextLog(ctx, "Switching to external camera")

	if err := utils.ControlFixture(ctx, extCameraID, "on"); err != nil {
		s.Fatal("Failed to connect external camera: ", err)
	}

	if _, err := getExternalCameraID(ctx, dut); err != nil {
		s.Fatal("Failed to check external camera: ", err)
	}

	if err := utils.ClickOnMaximizeButton(ctx, uiautoSvc, "Camera"); err != nil {
		s.Fatal("Failed to click on maximize button: ", err)
	}

	if err := switchToExternalCamera(ctx, dut, uiautoSvc); err != nil {
		s.Fatal("Failed to switch Camera app to external camera: ", err)
	}

	testing.ContextLog(ctx, "Verify by detecting the camera app view is red")

	fullscreenshotFile, err := utils.TakeWindowScreenshot(ctx, uiautoSvc, fs, utils.CameraWindowFinder)
	if err != nil {
		s.Fatal("Failed to take a full screen screenshot on the build-in display: ", err)
	}

	screenshotFile := filepath.Join(utils.DownloadsPath, fullscreenshotFile.Name())
	savedFile, err := utils.CopyRemoteFile(ctx, fs, screenshotFile, s.OutDir())
	if err != nil {
		s.Fatal("Failed to copy remote file to local host: ", err)
	}

	if err := utils.ValidateImgColor(ctx, savedFile, "red"); err != nil {
		s.Fatal("Failed to validate image color: ", err)
	}

}

func launchCameraApp(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient) error {
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch camera app")
	}

	cameraAppWindowFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Camera"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: cameraAppWindowFinder}); err != nil {
		return errors.Wrap(err, "failed to wait until the camera app exists")
	}

	// Since camera app takes time to preview camera screen, so need to wait for the window become stable.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := uiautoSvc.WaitForLocation(ctx, &ui.WaitForLocationRequest{Finder: cameraAppWindowFinder}); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}

func switchToExternalCamera(ctx context.Context, dut *dut.DUT, uiautoSvc ui.AutomationServiceClient) error {
	switchDeviceButtonFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Switch to next camera"}},
			{Value: &ui.NodeWith_Focusable{}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: switchDeviceButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to wait for switch button from context menu")
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: switchDeviceButtonFinder}); err != nil {
		return errors.Wrap(err, "failed to to click switch button from context menu")
	}

	activeInfoRegex := ".*?active"
	if err := utils.FindInfoOnCameraApp(ctx, uiautoSvc, activeInfoRegex); err != nil {
		return errors.Wrap(err, "failed to to wait for external camera to be active")
	}

	return nil
}

// getExternalCameraID is for get external camera id.
func getExternalCameraID(ctx context.Context, dut *dut.DUT) (string, error) {
	var externalCameraID string
	regex := `((?i)camera|(?i)webcam)`
	expMatch := regexp.MustCompile(regex)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "sudo", "lsusb").Output()
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to retrieve lsusb info from DUT"))
		}

		for _, usbDevice := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if expMatch.MatchString(string(usbDevice)) {
				regexID := `\w{4}:\w{4}`
				expMatchID := regexp.MustCompile(regexID)
				if expMatchID.MatchString(usbDevice) {
					externalCameraID = expMatchID.FindString(usbDevice)
					return nil
				}
			}

		}

		return errors.Errorf("Unable to find external camera info %q: ", out)

	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return "", err
	}
	return externalCameraID, nil
}
