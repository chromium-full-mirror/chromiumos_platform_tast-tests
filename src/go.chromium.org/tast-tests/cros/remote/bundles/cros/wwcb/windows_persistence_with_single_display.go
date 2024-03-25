// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

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

const (
	filesWindowTitle  = "Files - My files"
	chromeWindowTitle = "Chrome - New Tab"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WindowsPersistenceWithSingleDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test windows persistent settings with the single external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.wwcb.DisplayService", "tast.cros.apps.AppsService", "tast.cros.browser.ChromeService"},
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func WindowsPersistenceWithSingleDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	extDispID := s.RequiredVar("ExtDispID1")

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

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect external display: ", err)
	}

	if dockingID, ok := s.Var("DockingID"); ok {
		// Open IP power to supply docking power.
		ipPowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
			s.Fatal("Failed to open IP power: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

		if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
			s.Fatal("Failed to connect docking station: ", err)
		}
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	// Open two apps on external display.
	if err := openAppsOnExternalDisplay(ctx, appsSvc, displaySvc); err != nil {
		s.Fatal("Failed to open two apps on external display: ", err)
	}

	// Unplug and re-plug in, check windows on expected display.
	if err := unplugAndReplug(ctx, displaySvc, extDispID); err != nil {
		s.Fatal("Failed to unplug and replug external display: ", err)
	}

	// Test primary mode, check windows on expected display.
	if err := testPrimaryMode(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test primary mode: ", err)
	}

	// Unplug and re-plug in, check windows on expected display.
	if err := unplugAndReplug(ctx, displaySvc, extDispID); err != nil {
		s.Fatal("Failed to unplug and replug external display: ", err)
	}

	// Test mirror mode, check display attributes.
	if err := testMirrorMode(ctx, displaySvc); err != nil {
		s.Fatal("Failed to test mirror mode: ", err)
	}
}

func openAppsOnExternalDisplay(ctx context.Context, appsSvc pb.AppsServiceClient, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Open two apps on external display")

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Files", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch files app")
	}

	if _, err := appsSvc.LaunchPrimaryBrowser(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to launch primary browser")
	}

	// Retry to switch window, testing would fail on getting display info sometimes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: filesWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch files window to external display")
		}

		if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1, WindowTitle: chromeWindowTitle}); err != nil {
			return errors.Wrap(err, "failed to switch chrome window to external display")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to switch two windows to external display")
	}
	return nil
}

func unplugAndReplug(ctx context.Context, displaySvc wwcb.DisplayServiceClient, extDispID string) error {
	testing.ContextLog(ctx, "Unplug and re-plug in, check windows on expected display")

	if err := utils.ControlFixture(ctx, extDispID, "off"); err != nil {
		return errors.Wrap(err, "failed to disconnect external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify files window on internal display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: chromeWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to verify chrome window on internal display")
	}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		return errors.Wrap(err, "failed to connect external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify files window on external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: chromeWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify chrome window on external display")
	}
	return nil
}

func testPrimaryMode(ctx context.Context, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Test primary mode")

	if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to set internal display as primary")
	}

	if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to switch files window to internal display")
	}

	if _, err := displaySvc.SwitchWindowToDisplay(ctx, &wwcb.QueryRequest{WindowTitle: chromeWindowTitle, DisplayIndex: 0}); err != nil {
		return errors.Wrap(err, "failed to switch chrome window to internal display")
	}

	if _, err := displaySvc.SetPrimaryDisplay(ctx, &wwcb.QueryRequest{DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to set external display as primary")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: filesWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify files window on external display")
	}

	if _, err := displaySvc.VerifyWindowOnDisplay(ctx, &wwcb.QueryRequest{WindowTitle: chromeWindowTitle, DisplayIndex: 1}); err != nil {
		return errors.Wrap(err, "failed to verify chrome window on external display")
	}
	return nil
}

func testMirrorMode(ctx context.Context, displaySvc wwcb.DisplayServiceClient) error {
	testing.ContextLog(ctx, "Test mirror mode")
	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: true}); err != nil {
		return errors.Wrap(err, "failed to enable mirror display")
	}

	if _, err := displaySvc.SetMirrorDisplay(ctx, &wwcb.QueryRequest{Enable: false}); err != nil {
		return errors.Wrap(err, "failed to disable mirror display")
	}
	return nil
}
