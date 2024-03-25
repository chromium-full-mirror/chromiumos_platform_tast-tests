// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"google.golang.org/grpc"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlugExternalStorageWhileSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that when suspending with an external storage device connected, upon device wake-up, it can correctly copy files",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService"},
		Data:         []string{"sample.txt"},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			// Turn off table mode.
			Val: false,
		}, {
			Name: "tablet_mode",
			// Turn on tablet mode.
			Val: true,
		}},
		// Maximum timeout for data transfer in bidirectional and cleanup.
		Timeout: 5 * time.Minute,
	})
}
func PlugExternalStorageWhileSuspend(ctx context.Context, s *testing.State) {
	/*
		Test suspending with an external storage connected, it can correctly copy file:
		1- Initfixture.
		2- Plug external storage.
		3- Unplug external storage.
		4- Suspend chrome.
		5- Plug external storage.
		6- Resume chrome.
		7- Login chrome.
		8- Push file to remote.
		9- Launch the files app.
		10- Copy file to external storage and verify file are being copied successfully.
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	usbID := s.RequiredVar("USBID")
	testParms := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to initialize servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	tablet := isDUTInTabletMode(ctx, s, dut)
	// Enable table mode.
	if testParms && !tablet {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to initialize to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	} else if !testParms && tablet {
		testing.ContextLog(ctx, "Disable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode off"); err != nil {
			s.Fatal("Failed to initialize to disable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode on")
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
		s.Fatal("Failed to initialize to connect the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Login chrome.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize to chrome login: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Unplug external storage.
	if err := utils.ControlFixture(ctx, usbID, "off"); err != nil {
		s.Fatal("Failed to unplug the external storage after login chrome: ", err)
	}
	// Suspend chrome.
	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		s.Fatal("Failed to suspend after login chrome and unplug external storage: ", err)
	}
	// Plug external storage.
	if err := utils.ControlFixture(ctx, usbID, "on"); err != nil {
		s.Fatal("Failed to plug the external storage after suspend: ", err)
	}
	// Resume chrome.
	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to resume after plug external storage: ", err)
	}

	// Reconnect to the gRPC server on the DUT.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize to connect the RPC service on the DUT after resume: ", err)
	}

	// Login chrome.
	cs = ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize to chrome login after resume: ", err)
	}

	// Push file to remote.
	remoteTextPath, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
	if err != nil {
		s.Fatal("Failed to initialize to push file to DUT's /tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTextPath).Output()

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	// Launch the Files app.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app after resume: ", err)
	}

	mountPoints, err := utils.GetMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get mount points: ", err)
	}
	for _, mountPoint := range mountPoints {
		usbTextPath := filepath.Join(mountPoint, "sample.txt")
		// Copy file to external storage.
		if err := utils.CopyFileToExternalStorage(ctx, dut, mountPoint, remoteTextPath); err != nil {
			s.Fatal("Failed to copy file to external storage: ", err)
		}
		// Check external storage not read only.
		if readOnly, err := utils.GetDeviceWritableStatus(ctx, dut, mountPoint); err != nil {
			s.Fatal("Failed to get device writable status: ", err)
		} else if !readOnly {
			if err := utils.CompareTwoFiles(ctx, dut, remoteTextPath, usbTextPath); err != nil {
				s.Fatal("Failed to comparison of the dut file with the usb file after open files app: ", err)
			}
		}
	}

	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Files app perhaps due to an app crash: ", err)
	}
}

// isDUTInTabletMode returns true if the DUT is in tablet mode, false otherwise.
func isDUTInTabletMode(ctx context.Context, s *testing.State, dut *dut.DUT) bool {
	// Verify powerd received the tablet mode change notification.
	testing.ContextLog(ctx, "Get the tabletmode value from powerd")
	// Expected output from the `dbus-send` command is:
	//    boolean true
	// OR
	//    boolean false
	out, err := dut.Conn().CommandContext(ctx, "dbus-send", "--system", "--print-reply=literal", "--type=method_call", "--dest=org.chromium.PowerManager", "/org/chromium/PowerManager", "org.chromium.PowerManager.GetTabletMode").Output()
	if err != nil {
		s.Fatal("Failed to retrieve dbus-send output: ", err)
	}
	words := strings.Fields(string(out))
	if len(words) != 2 {
		s.Fatal("Received unexpected output from dbus-send: ", out)
	}

	testing.ContextLog(ctx, "powerd tabletmode: ", words[1])
	return words[1] == "true"
}
