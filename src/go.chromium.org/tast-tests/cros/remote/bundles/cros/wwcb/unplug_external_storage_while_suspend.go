// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/servo"
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
		Func:         UnplugExternalStorageWhileSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that when suspending with an external storage device disconnected, upon device wake-up, it can correctly copy files",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService"},
		Data:         []string{"sample.txt"},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			// Disable tablet mode
			Val: false,
		}, {
			Name: "tablet_mode",
			// Enable tablet mode
			Val: true,
		}},
		// Maximum timeout for data transfer in bidirectional and cleanup.
		Timeout: 5 * time.Minute,
	})
}
func UnplugExternalStorageWhileSuspend(ctx context.Context, s *testing.State) {
	/*
		Test when suspending with an external storage device disconnected, upon device wake-up, it can correctly copy files:
		1- InitFixture.
		2- Plug external storage.
		3- Login chrome.
		4- Suspend chrome.
		5- Unplug external storage.
		6- Resume chrome.
		7- Login chrome.
		8- Plug external storage.
		9- Push file to remote.
		10- Launch the files app.
		11- Copy file to external storage and verify file are being copied successfully.
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	usbID := s.RequiredVar("USBID")
	enableTabletMode := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to initialize servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Enable tablet mode.
	if enableTabletMode {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	// Initfixture.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}

	// Plug external storage.
	if err := utils.ControlFixture(ctx, usbID, "on"); err != nil {
		s.Fatal("Failed to connect to the external storage: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Login chrome.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Suspend chrome.
	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		s.Fatal("Failed to suspend after login chrome: ", err)
	}
	// Unplug external storage.
	if err := utils.ControlFixture(ctx, usbID, "off"); err != nil {
		s.Fatal("Failed to unplug the external storage after suspend: ", err)
	}
	// Resume chrome.
	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to resume after unplugging external storage: ", err)
	}

	// Reconnect to the gRPC server on the DUT.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after resume: ", err)
	}

	// Login chrome.
	cs = ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to login to chrome after resume: ", err)
	}

	beforeMountPoints, err := utils.RemovableMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get mountpoints external storage USB Device: ", err)
	}
	s.Log("Following mount points were found prior to plugging in external storage USB Device: ", beforeMountPoints)

	// Plug external storage.
	if err := utils.ControlFixture(ctx, usbID, "on"); err != nil {
		s.Fatal("Failed to plug the external storage after resume: ", err)
	}

	// Push file to remote.
	remoteTextPath, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
	if err != nil {
		s.Fatal("Failed to push file to DUT's /tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTextPath).Output()

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	// Launch the Files app.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app after resume: ", err)
	}

	afterMountPoints, err := utils.RemovableMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get mountpoints after connecting external storage USB Device: ", err)
	}
	s.Log("Following mount points were found after plugging in external storage USB device: ", afterMountPoints)

	mountPoints := utils.FindDifference(afterMountPoints, beforeMountPoints)
	s.Log("Following mount points are new and identified as the external storage USB device and will be used: ", mountPoints)

	for _, mountPoint := range mountPoints {
		usbTextPath := filepath.Join(mountPoint, "sample.txt")
		// Copy file to external storage.
		if err := utils.CopyFileToExternalStorage(ctx, dut, mountPoint, remoteTextPath); err != nil {
			s.Fatal("Failed to copy file to external storage: ", err)
		}
		defer dut.Conn().CommandContext(cleanupCtx, "rm", filepath.Join(mountPoint, "sample.txt")).Output()
		// Check external storage not read only.
		if readOnly, err := utils.GetDeviceWritableStatus(ctx, dut, mountPoint); err != nil {
			s.Fatal("Failed to get device writable status: ", err)
		} else if !readOnly {
			if err := utils.CompareTwoFiles(ctx, dut, remoteTextPath, usbTextPath); err != nil {
				s.Fatal("Failed to compare of the dut file with the usb file after opening files app: ", err)
			}
		}
	}

	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Files app perhaps due to an app crash: ", err)
	}
}
