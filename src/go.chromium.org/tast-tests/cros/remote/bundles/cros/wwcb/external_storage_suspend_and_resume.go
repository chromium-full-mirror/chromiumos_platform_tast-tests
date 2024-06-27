// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
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
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalStorageSuspendAndResume,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that the Device, when powered on while connected to the DUT and after undergoing Suspend/Resume, has its filesystems, FileManager, and ARC++ Application working as expected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo"},
		Fixture:      "wwcbStorage",
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.ChromeUIService"},
		Data:         []string{"sample.txt"},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}},
	})
}

func ExternalStorageSuspendAndResume(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testParms := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Enable table mode.
	if testParms {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	before, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get original USB devices: ", err)
	}

	// Plug in the USB devices.
	tf := s.FixtValue().(*topology.TestFixture)
	if _, err := tf.Helper.ActivateDeviceByType(ctx, topology.DeviceTypeStorage); err != nil {
		s.Fatal("Failed to control fixture to connect the external storage media: ", err)
	}

	// Reboot the DUT to simulate inserting External Storage while the DUT is powered off.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot the DUT before sign-in account: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after power the Chrombook on: ", err)
	}

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start chrome after power the chrombook on: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	if err := utils.SuspendDUT(ctx, s.DUT(), pxy); err != nil {
		s.Fatal("Failed to perform powerdbus suspend after sign-in account: ", err)
	}

	if err := dut.WaitUnreachable(ctx); err != nil {
		s.Fatal("Failed to wait DUT to be unreachable after suspend: ", err)
	}

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to wake DUT after suspend: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after resume: ", err)
	}
	defer cl.Close(cleanupCtx)

	cs = ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start chrome after resume: ", err)
	}
	// GoBigSleepLint: Sleep a little bit to ensure logging in.
	testing.Sleep(ctx, 5*time.Second)

	// Launch the Files app.
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app after resume: ", err)
	}

	// Expect number of USB devices is not less than number of input parameters.
	if err := utils.VerifyUSBDeviceConnectionChangeCount(ctx, dut, len(before), 1); err != nil {
		s.Fatal("Failed to match the expected number of USB devices after resume: ", err)
	}

	// Push file to remote.
	remoteTXTPath, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
	if err != nil {
		s.Fatal("Failed to push file to DUT's /tmp directory after check all partitions are mounted: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTXTPath).Output()

	// Retrieve USB path.
	mountPoints, err := utils.GetMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get the original USB devices after check all partitions are mounted: ", err)
	}

	var mountPoint string
	if len(mountPoints) >= 1 {
		mountPoint = mountPoints[0]
	} else {
		s.Fatalf("Failed to proceed due to incorrect number of mountPoints, got %d want at least 1", len(mountPoints))
	}

	// Copy file to storage media.
	if err := utils.CopyFileToExternalStorage(ctx, dut, mountPoint, remoteTXTPath); err != nil {
		s.Fatal("Failed to copy or compare files between DUT and USB due to an error after check all partitions are mounted: ", err)
	}
	defer dut.Conn().CommandContext(ctx, "rm", mountPoint).Output()

	// Get the read-write protection status of the device.
	mountPointStatus, err := utils.GetDeviceWritableStatus(ctx, dut, mountPoint)
	if err != nil {
		s.Fatal("Failed to obtain the read-write status of the device after copy file to storage media: ", err)
	}

	// Compare the files between DUT and storage media.
	if mountPointStatus == true {
		usbTextPath := filepath.Join(mountPoint, "sample.txt")
		if err := utils.CompareTwoFiles(ctx, dut, remoteTXTPath, usbTextPath); err != nil {
			s.Fatal("Failed to compare files between DUT and USB due to an error after copy file to storage media: ", err)
		}
	}

	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close the Files app after checking if files were copied properly: ", err)
	}
}
