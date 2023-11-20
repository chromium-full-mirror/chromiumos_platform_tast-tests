// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CordedMouseButtonFunctionalityReconnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the functionality of the mouse buttons after reconnect",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.AutomationService"},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func CordedMouseButtonFunctionalityReconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	USBID := s.RequiredVar("USBID")

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
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to turn on fixture to connect the corded mouse: ", err)
	}

	// Retrieve the mouse device event and verify its count.
	mouseList, err := utils.GetDeviceEventMouse(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to retrieve the devices event mouse: ", err)
	} else if len(mouseList) != 1 {
		s.Fatalf("Failed to proceed due to incorrect number of device event mouse, expect: 1, actual: %d", len(mouseList))
	}

	// Plug out the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "off"); err != nil {
		s.Fatal("Failed to turn off fixture to connect the corded mouse: ", err)
	}

	// Retrieve the mouse device event and verify its count.
	mouseList, err = utils.GetDeviceEventMouse(ctx, s.DUT())
	if err == nil && len(mouseList) != 0 {
		s.Fatalf("Failed to proceed due to incorrect number of device event mouse after plug out, expect: 0, actual: %d", len(mouseList))
	}

	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to turn on fixture to connect the corded mouse after plug out: ", err)
	}

	// Retrieve the mouse device event and verify its count.
	mouseList, err = utils.GetDeviceEventMouse(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to retrieve the devices event mouse: ", err)
	} else if len(mouseList) != 1 {
		s.Fatalf("Failed to proceed due to incorrect number of device event mouse after plug in, expect: 1, actual: %d", len(mouseList))
	}

	device := "/dev/input/by-id/" + mouseList[0]

	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(ctx)

	if err := utils.CheckMouseButton(ctx, s, device); err != nil {
		s.Fatal("Failed to check mouse button: ", err)
	}
}
