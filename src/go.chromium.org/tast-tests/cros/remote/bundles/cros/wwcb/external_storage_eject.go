// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
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
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.wwcb.ExternalStorageService"},
		Data:         []string{"sample.txt"},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func ExternalStorageEject(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	USBID := s.RequiredVar("USBID")

	// Set up the servo attached to the DUT.
	dut := s.DUT()

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

	// Retrieve removable block devices' mount points.
	if _, err = utils.GetMountPoints(ctx, dut); err != nil {
		s.Fatal("Failed to get USB devices after sign-in account: ", err)
	}

	// Eject all removable block devices.
	externalStorageSvc := wwcb.NewExternalStorageServiceClient(cl.Conn)
	_, err = externalStorageSvc.EjectAll(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to eject USB device: ", err)
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
