// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
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

	s.AttachErrorHandlers(
		func(errMsg string) {
			utils.DumpUITreeWithScreenshotToFile(cleanupCtx, cl.Conn, "ui_dump_error")
		},
		func(errMsg string) {
			utils.DumpUITreeWithScreenshotToFile(cleanupCtx, cl.Conn, "ui_dump_fatal")
		},
	)

	// Check USB webcam can be detect properly (lsusb, dmesg, etc...).
	builtinDevices, err := utils.DevicesFromV4L2(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get built-in devices from V4L2: ", err)
	}
	if len(builtinDevices) == 0 {
		s.Fatal("Expect to get at least one built-in device, but get nothing")
	}
	testing.ContextLog(ctx, "Found built-in camera: ", builtinDevices)

	// Initialize fixtures to find the connected devices.
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
		s.Fatal("Failed to connect external camera: ", err)
	}
	testing.ContextLogf(ctx, "Found external camera: %s", extCamera)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Camera", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Camera", TimeoutSecs: 60})

	if err := utils.WaitForFinderLocationStable(ctx, uiautoSvc, utils.CameraWindowFinder); err != nil {
		s.Fatal("Failed to wait for camera window to be stabled: ", err)
	}

	// Check CCA app is using built-in camera.
	current, err := utils.CCAUseDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to retrieve CCA is using which camera: ", err)
	}
	if !utils.Contains(builtinDevices, current) {
		s.Fatalf("Expect CCA app is using built-in camera: %v, but using %s", builtinDevices, current)
	}

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
