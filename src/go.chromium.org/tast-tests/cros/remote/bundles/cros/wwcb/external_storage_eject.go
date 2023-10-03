// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalStorageEject,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the USB device after ejecting to ensure all partitions are mounted and the ARC++ application, i.e. Files app, is working as expected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService"},
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

func ExternalStorageEject(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	USBID := s.RequiredVar("USBID")

	tabletMode := s.Param().(bool)

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Enable tablet mode.
	if tabletMode {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "on").Run(); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer func() {
			dut = s.DUT()
			if err := dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "off").Run(); err != nil {
				s.Fatal("Failed to disable tablet mode: ", err)
			}
		}()
	}

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	mountPointsBeforePlugInUSB, err := utils.GetMountPoints(ctx, dut)
	if err == nil {
		s.Fatal("Failed to ensure no USB devices appeared: ", err)
	}

	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the external storage media: ", err)
	}

	// Reboot the DUT to simulate inserting External Storage while the DUT is powered off.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot the DUT before sign-in account: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after powering the chromebook on: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome after power the chromebook on: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	var mountPoints []string
	// Retrieve removable block devices' mount points.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		mountPoints, err = utils.GetMountPoints(ctx, dut)
		if err != nil {
			return errors.New("unable to obtain the mount point")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to get original USB devices after sign-in account: ", err)
	}

	// TODO in the future, services will replace shell command pipes.
	// Eject all removable block devices.
	for _, mountPoint := range mountPoints {
		output, err := dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("df | grep '%s' | awk '{print $1}' | head -n 1", mountPoint)).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to obtain device node before eject: ", err)
		}
		deviceNode := strings.TrimSpace(string(output))
		err = dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("sudo eject %s", deviceNode)).Run(testexec.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to eject USB device: ", err)
		}
	}

	// Check all partitions are unmounted.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, _ := utils.GetMountPoints(ctx, dut)
		if len(after)-len(mountPointsBeforePlugInUSB) != 0 {
			return errors.Errorf("unexpected change in the number of USB devices detected after eject; expect: %d, actual: %d (from %d to %d)", len(mountPointsBeforePlugInUSB), len(after), len(mountPointsBeforePlugInUSB), len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to detect the normal number of devices after eject: ", err)
	}

	// Unplug the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "off"); err != nil {
		s.Fatal("Failed to control fixture to disconnect the external storage media after eject: ", err)
	}

	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the external storage media after eject: ", err)
	}

	// Launch the Files app.
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch Files app after eject: ", err)
	}

	// Check all partitions are mounted.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, _ := utils.GetMountPoints(ctx, dut)
		if len(after)-len(mountPointsBeforePlugInUSB) == 0 {
			return errors.Errorf("unexpected change in the number of USB devices detected after eject; expect: %d, actual: %d (from %d to %d)", len(mountPointsBeforePlugInUSB), len(after), len(mountPointsBeforePlugInUSB), len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to detect the normal number of devices after eject: ", err)
	}

	// Close the Files app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close the Files app after eject: ", err)
	}
}
