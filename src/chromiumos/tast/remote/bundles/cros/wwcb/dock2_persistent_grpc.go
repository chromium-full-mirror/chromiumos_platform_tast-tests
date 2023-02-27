// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook
package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/rpc"
	pb "chromiumos/tast/services/cros/apps"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wwcb"
	"chromiumos/tast/testing"
)

const (
	fileWindowTitle    = "Files - My files"
	galleryWindowTitle = "Gallery"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dock2PersistentGRPC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Windows persistent settings for dual display through a Dock",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112",
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"DockingID", "ExtDispID1", "ExtDispID2", "wwcbIPPowerIp"},
		ServiceDeps: []string{
			"tast.cros.wwcb.DisplayService",
			"tast.cros.apps.AppsService",
			"tast.cros.browser.ChromeService",
		},
	})
}

func Dock2PersistentGRPC(ctx context.Context, s *testing.State) {
	// Reboot the DUT, in case of DUT unable to work properly consistently.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot the DUT: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID1 := s.RequiredVar("ExtDispID1")
	extDispID2 := s.RequiredVar("ExtDispID2")

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
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

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)

	// Open IP power to supply docking power.
	if err := utils.OpenIppower(ctx, []int{1}); err != nil {
		s.Fatal("Failed to open IP power: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, []int{1})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := connectDisplaysViaDock(ctx, displaySvc, dockingID, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to connect to the two external displays via docking station: ", err)
	}

	if err := openAppsOnDualDisplay(ctx, appsSvc, displaySvc, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to open two apps on two external displays: ", err)
	}

	if err := replugExternalDisplay(ctx, displaySvc, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to replug two external displays: ", err)
	}

	if err := testPrimaryModeWithDualDisplay(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test primary mode on two external displays: ", err)
	}

	if err := replugExternalDisplayInPrimary(ctx, displaySvc, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to replug two external displays when the first external display is in primary: ", err)
	}

	if err := testMirrorModeWithDualDisplay(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test mirror mode with two external displays: ", err)
	}
}

func connectDisplaysViaDock(ctx context.Context, displaySvc wwcb.DisplayServiceClient, dockingID, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Connect to the two external displays via docking station")

	if err := utils.ControlFixture(ctx, extDispID1, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the first external display")
	}

	if err := utils.ControlFixture(ctx, extDispID2, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the second external display")
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the docking station")
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 3}); err != nil {
		return errors.Wrap(err, "failed to verify display count")
	}
	return nil
}

func openAppsOnDualDisplay(ctx context.Context, appsSvc pb.AppsServiceClient, displaySvc wwcb.DisplayServiceClient, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Open two apps on dual external displays")

	// Open two any app for testing requirement, just to get a window.
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch Files App")
	}

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch Gallery App")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: fileWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch Files App to the first external display")
		}

		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 2, WindowTitle: galleryWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch Gallery App to the second external display")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to switch the two windows to external display")
	}
	return nil
}

func replugExternalDisplay(ctx context.Context, displaySvc wwcb.DisplayServiceClient, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Unplug and replug in, check windows on expected display")

	if err := utils.ControlFixture(ctx, extDispID1, "off"); err != nil {
		return errors.Wrap(err, "failed to disconnect the first external display")
	}

	if err := utils.ControlFixture(ctx, extDispID2, "off"); err != nil {
		return errors.Wrap(err, "failed to disconnect the second external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on internal display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on internal display")
	}

	if err := utils.ControlFixture(ctx, extDispID1, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the first external display")
	}

	if err := utils.ControlFixture(ctx, extDispID2, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the second external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify the File App's window is on the first external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 2}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on the second external display")
	}
	return nil
}

func testPrimaryModeWithDualDisplay(ctx context.Context, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Test primary mode")

	if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to set internal display as primary")
	}

	// Retry to switch windows, because windows need to be switched twice then it will show on the internal display.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0, WindowTitle: fileWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch Files App to internal display")
		}

		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0, WindowTitle: galleryWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch Gallery App to internal display")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to switch windows to internal display")
	}

	if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to set external display as primary")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on external display")
	}
	return nil
}

func replugExternalDisplayInPrimary(ctx context.Context, displaySvc wwcb.DisplayServiceClient, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Unplug and replug in, check windows on expected display")

	if err := utils.ControlFixture(ctx, extDispID1, "off"); err != nil {
		return errors.Wrap(err, "failed to disconnect the first external display")
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		return errors.Wrap(err, "failed to verify display count")
	}

	if err := utils.ControlFixture(ctx, extDispID2, "off"); err != nil {
		return errors.Wrap(err, "failed to disconnect the second external display")
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 1}); err != nil {
		return errors.Wrap(err, "failed to verify display count")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on internal display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on internal display")
	}

	if err := utils.ControlFixture(ctx, extDispID1, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the first external display")
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		return errors.Wrap(err, "failed to verify display count")
	}

	if err := utils.ControlFixture(ctx, extDispID2, "on"); err != nil {
		return errors.Wrap(err, "failed to connect to the second external display")
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 3}); err != nil {
		return errors.Wrap(err, "failed to verify display count")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on external display")
	}
	return nil
}

func testMirrorModeWithDualDisplay(ctx context.Context, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Test mirror mode")

	if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to set internal display as primary")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on internal display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on internal display")
	}

	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: true}); err != nil {
		return errors.Wrap(err, "failed to enable mirror display")
	}

	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: false}); err != nil {
		return errors.Wrap(err, "failed to disable mirror display")
	}
	return nil
}
