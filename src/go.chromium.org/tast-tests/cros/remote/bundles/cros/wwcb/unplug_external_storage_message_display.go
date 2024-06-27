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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnplugExternalStorageMessageDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify if a warning message is displayed after unplugging the external storage, and ensure that Files app does not crash",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo"},
		Fixture:      "wwcbStorage",
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.ui.ChromeUIService"},
		Data:         []string{"sample.txt"},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}},
		// Maximum timeout for data transfer in bidirectional and cleanup.
		Timeout: 5 * time.Minute,
	})
}
func UnplugExternalStorageMessageDisplay(ctx context.Context, s *testing.State) {
	/*
		Test warning message is displayed after unplugging external storage,and Files app does not crash:
		1- Login to chrome.
		2- Push file to remote dut.
		3- Initfixture.
		4- Launch files app.
		5- Plug external storage.
		6- Copy and compare file.
		7- Unplug external storage.
		8- Check warning message is displayed.
		9- Close files app ensure does not crash.
	*/
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	isTablet := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Enable table mode.
	if isTablet {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to login Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Push file to remote.
	remoteTextPath, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
	if err != nil {
		s.Fatal("Failed to push file to DUT's /tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTextPath).Output()

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	// Launch the Files app.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	tf := s.FixtValue().(*topology.TestFixture)
	usbID, err := tf.Helper.ActivateDeviceByType(ctx, topology.DeviceTypeStorage)
	if err != nil {
		s.Fatal("Failed to connect to the external storage: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		mountPoints, err := utils.GetMountPoints(ctx, dut)
		if err != nil {
			errors.Wrap(err, "get mount points")
		}
		for _, mountPoint := range mountPoints {
			usbTextPath := filepath.Join(mountPoint, "sample.txt")
			// Copy file to external storage.
			if err := utils.CopyFileToExternalStorage(ctx, dut, mountPoint, remoteTextPath); err != nil {
				errors.Wrap(err, "copy file to external storage")
			}
			// Check external storage not read only.
			if readOnly, err := utils.GetDeviceWritableStatus(ctx, dut, mountPoint); err != nil {
				errors.Wrap(err, "get device writable status")
			} else if !readOnly {
				if err := utils.CompareTwoFiles(ctx, dut, remoteTextPath, usbTextPath); err != nil {
					errors.Wrap(err, "comparison of the dut file with the usb file")
				}
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second}); err != nil {
		s.Fatal("Failed to check external storage file: ", err)
	}
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	// Unplug the USB devices.
	go func() {
		if err := tf.Helper.DeactivateDeviceByID(ctx, usbID); err != nil {
			s.Fatal("Failed to unplug the external storage after copy file: ", err)
		}
	}()
	// Waiting for the system to display the "Whoa, there. Be careful." message.
	notificationMessage := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_ALERT_DIALOG}},
			{Value: &ui.NodeWith_NameRegex{NameRegex: "Whoa, there. Be careful."}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: notificationMessage}); err != nil {
		s.Fatal("Failed to Wait Whoa, there. Be careful. message after unplug external storage: ", err)
	}
	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Files app perhaps due to an app crash: ", err)
	}
}
