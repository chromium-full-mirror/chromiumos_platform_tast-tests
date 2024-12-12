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
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	fileWindowTitle    = "Files - My files"
	galleryWindowTitle = "Gallery"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowsPersistenceWithDualDisplay,
		Desc:         "Test Windows persistent settings with dual external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr: []string{
			"group:wwcb",
			"group:pasit",
			"group:release-health",
			"release-health_usb",
		},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"DockingID", "newTestItem"},
		ServiceDeps:  []string{"tast.cros.wwcb.DisplayService", "tast.cros.apps.AppsService", "tast.cros.browser.ChromeService", "tast.cros.ui.ChromeUIService"},
		TestBedDeps:  []string{"pasit_components:MONITOR-2"},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcb.dock",
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func WindowsPersistenceWithDualDisplay(ctx context.Context, s *testing.State) {
	// Reboot the DUT, in case of DUT unable to work properly consistently.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot the DUT: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	tf := s.FixtValue().(*topology.TestFixture)
	displays := tf.Helper.DevicesByType(topology.DeviceTypeMonitor)
	if len(displays) < 2 {
		s.Fatalf("Failed to find required displays, expected at least 2, found: %d", len(displays))
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}
	}

	extDispID1 := displays[0]
	if err := tf.Helper.ActivateDeviceByID(ctx, extDispID1); err != nil {
		s.Fatal("Failed to connect to the first external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	twoDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	}

	extDispID2 := displays[1]
	if err := tf.Helper.ActivateDeviceByID(ctx, extDispID2); err != nil {
		s.Fatal("Failed to connect to the second external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 3}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	threeDisplays, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	}

	// Get the display info about connecting to the external display each time.
	// Use index [1] as comparision. When the external display is plugged in twice, index [1] can be obtained.
	// If index [1] of two lists are the same, means the sequence of external display detected by system is as same as input parameter.
	// If not, means that sequence of both external display are different.
	if twoDisplays.DisplayIds[1] != threeDisplays.DisplayIds[1] {
		testing.ContextLog(ctx, "Switch the sequence of external display 1 & 2")
		extDispID1, extDispID2 = extDispID2, extDispID1
	}

	if err := openAppsOnDualDisplay(ctx, appsSvc, displaySvc); err != nil {
		s.Fatal("Failed to open two apps on two external displays: ", err)
	}

	if err := replugExternalDisplay(ctx, displaySvc, tf.Helper, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to replug two external displays: ", err)
	}

	if err := testPrimaryModeWithDualDisplay(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test primary mode on two external displays: ", err)
	}

	if err := replugExternalDisplayInPrimary(ctx, displaySvc, tf.Helper, extDispID1, extDispID2); err != nil {
		s.Fatal("Failed to replug two external displays when the first external display is in primary: ", err)
	}

	if err := testMirrorModeWithDualDisplay(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test mirror mode with two external displays: ", err)
	}
}

func openAppsOnDualDisplay(ctx context.Context, appsSvc pb.AppsServiceClient, displaySvc wwcb.DisplayServiceClient) error {
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

func replugExternalDisplay(ctx context.Context, displaySvc wwcb.DisplayServiceClient, helper *topology.Helper, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Unplug and replug in, check windows on expected display")

	if err := helper.DeactivateDeviceByID(ctx, extDispID1); err != nil {
		return errors.Wrap(err, "failed to disconnect the first external display")
	}

	if err := helper.DeactivateDeviceByID(ctx, extDispID2); err != nil {
		return errors.Wrap(err, "failed to disconnect the second external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Files App's window is on internal display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify the Gallery App's window is on internal display")
	}

	if err := helper.ActivateDeviceByID(ctx, extDispID1); err != nil {
		return errors.Wrap(err, "failed to connect to the first external display")
	}

	if err := helper.ActivateDeviceByID(ctx, extDispID2); err != nil {
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

	return nil
}

func replugExternalDisplayInPrimary(ctx context.Context, displaySvc wwcb.DisplayServiceClient, helper *topology.Helper, extDispID1, extDispID2 string) error {
	testing.ContextLog(ctx, "Unplug and replug in, check windows on expected display")

	for _, outer := range []struct {
		dispIndex int
		fixtureID string
	}{
		{1, extDispID1},
		{2, extDispID2},
	} {
		testing.ContextLogf(ctx, "Set external display %d as primary", outer.dispIndex)

		if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: int32(outer.dispIndex)}); err != nil {
			return errors.Wrapf(err, "failed to set external display %d as primary", outer.dispIndex)
		}

		if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: int32(outer.dispIndex)}); err != nil {
			return errors.Wrapf(err, "failed to verify the Files App's window is on external display: %d", int32(outer.dispIndex))
		}

		if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: int32(outer.dispIndex)}); err != nil {
			return errors.Wrapf(err, "failed to verify the Gallery App's window is on external display: %d", int32(outer.dispIndex))
		}

		// Unplug and re-plug in the external display, then check the windows bound on which display.
		for _, inner := range []struct {
			onOff           bool
			windowOnDisplay int
		}{
			{false, 0},
			{true, outer.dispIndex},
		} {

			if inner.onOff {
				if err := helper.ActivateDeviceByID(ctx, outer.fixtureID); err != nil {
					return errors.Wrap(err, "failed to turn on the fixture of the external display")
				}
			} else {
				if err := helper.DeactivateDeviceByID(ctx, outer.fixtureID); err != nil {
					return errors.Wrap(err, "failed to turn off the fixture of the external display")
				}
			}

			if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: fileWindowTitle, DisplayIndex: int32(inner.windowOnDisplay)}); err != nil {
				return errors.Wrapf(err, "failed to verify the Files App's window is on the display index: %d", inner.windowOnDisplay)
			}

			if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: galleryWindowTitle, DisplayIndex: int32(inner.windowOnDisplay)}); err != nil {
				return errors.Wrapf(err, "failed to verify the Gallery App's window is on the display index: %d", inner.windowOnDisplay)
			}
		}
	}
	return nil
}

func testMirrorModeWithDualDisplay(ctx context.Context, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Test mirror mode")

	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: true}); err != nil {
		return errors.Wrap(err, "failed to enable mirror display")
	}

	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: false}); err != nil {
		return errors.Wrap(err, "failed to disable mirror display")
	}
	return nil
}
