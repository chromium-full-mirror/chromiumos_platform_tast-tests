// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalStorageFormat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check whether the USB Device is working as expected in terms of filesystem and FileManager after formatting, and ensure the ARC++ application is working as expected",
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

func ExternalStorageFormat(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tabletMode := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Enable table mode.
	if tabletMode {
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

	before, err := utils.RemovableMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get mount points prior to plugging in new USB devices: ", err)
	}

	s.Log("Mount points prior to plugging in USB devices: ", before)

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
		s.Fatal("Failed to connect to the RPC service on the DUT after reboot: ", err)
	}

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome after reboot: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	var remoteTXTPath string
	// Push file to remote.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		remoteTXTPath, err = utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
		if err != nil {
			return errors.New("unable to push file to remote")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to push file to DUT's /tmp directory after sign-in account: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTXTPath).Output()

	// Retrieve USB path.
	afterMountPoints, err := utils.RemovableMountPoints(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get mount points after plugging in USB devices: ", err)
	}
	s.Log("Mount points after plugging in USB devices: ", afterMountPoints)

	mountPoints := utils.FindDifference(afterMountPoints, before)
	s.Log("Found following new mount points: ", mountPoints)

	mountPoint := mountPoints[0]

	s.Logf("Using %s as mount point for newly plugged in USB device", mountPoint)

	output, err := dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("df | grep '%s' | awk '{print $1}' | head -n 1", mountPoint)).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to obtain device node: ", err)
	}
	deviceNode := strings.TrimSpace(string(output))

	mountPointWritable, err := utils.GetDeviceWritableStatus(ctx, dut, mountPoint)
	if err != nil {
		s.Fatal("Failed to obtain the read-write status of the device: ", err)
	}

	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	fs := dutfs.NewClient(d.RPC().Conn)

	err = utils.FormatStorageToFAT(ctx, mountPoint, dut, fs)
	if mountPointWritable && err != nil {
		s.Fatal("Failed to format the device: ", err)
	} else if !mountPointWritable && err == nil {
		s.Fatal("Write-protected device should not be formatted")
	}
	if !mountPointWritable {
		// Mount device.
		if dirExists, err := fs.Exists(ctx, mountPoint); err != nil {
			s.Fatal("Failed to check the existence of the USB device's original mount point: ", err)
		} else if dirExists == false {
			if err := fs.MkDir(ctx, mountPoint, os.FileMode(0750)); err != nil {
				s.Fatalf("Failed to mkdir %s: %v", mountPoint, err)
			}
		}
		cmd := fmt.Sprintf("sudo mount %s '%s'", deviceNode, mountPoint)
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Failed to mount %s to DUT after format: %v", deviceNode, err)
		}
	}

	// Check all partitions are mounted.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, _ := utils.GetMountPoints(ctx, dut)
		if len(after)-len(before) != 1 {
			return errors.Errorf("unexpected change in the number of usb devices detected after format; expect: %d, actual: %d (from %d to %d)", 1, len(after), len(before), len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to detect the normal number of devices after format: ", err)
	}

	// Launch the Files app.
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app after format: ", err)
	}

	// Copy file to storage media.
	if err := utils.CopyFileToExternalStorage(ctx, dut, mountPoint, remoteTXTPath); err != nil {
		s.Fatal("Failed to copy or compare files between DUT and USB due to an error after check all partitions are mounted: ", err)
	}
	defer dut.Conn().CommandContext(ctx, "rm", mountPoint).Output()

	// Compare the files between DUT and storage media.
	if mountPointWritable {
		usbTextPath := filepath.Join(mountPoint, "sample.txt")
		if err := utils.CompareTwoFiles(ctx, dut, remoteTXTPath, usbTextPath); err != nil {
			s.Fatal("Failed to compare files between DUT and USB due to an error after copy file to storage media: ", err)
		}
	}

	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close the Files app after format: ", err)
	}
}
