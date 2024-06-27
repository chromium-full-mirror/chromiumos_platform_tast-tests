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
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UsbKeyboardFunctionalWithGuest,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check keyboard input and verify that the function is working properly in guest mode",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_hid"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
		},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func UsbKeyboardFunctionalWithGuest(ctx context.Context, s *testing.State) {
	/*
		Test steps:
		1. Boot and loging to ChromeOS.
		2. Connect a USB keyboard to the Chromebook USB port.
		3. Signout the Chromebook.
		4. Sign-in to the guest mode of the Chromebook.
		5. Click on all top row function keys on the USB keyboard.
		6. Click on all special keys on the USB keyboard.
		(Exception: Toggling Num Lock not supported)
		7. Open a document then input all of keys on the USB keyboard.
		8. Set the Chromebook into Tablet Mode and repeat the above steps 1~7.
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	USBID := s.RequiredVar("USBID")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after power the chrombook on: ", err)
	}
	defer cl.Close(cleanupCtx)
	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start chrome after power the chrombook on: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Plug in the USB devices.
	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the corded keyboard: ", err)
	}
	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(cleanupCtx)
	// Get device event kbd.
	kbdList, err := utils.GetDeviceEventKbd(ctx, dut)
	if err != nil {
		s.Fatal("Failed to retrieve the devices event kbd: ", err)
	} else if len(kbdList) != 1 {
		s.Fatalf("Failed to proceed due to incorrect number of device event kbd, expect: 1, actual: %d", len(kbdList))
	}

	if _, err := cs.Close(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to close chrome session: ", err)
	}
	// Start guest Chrome on the DUT.
	loginReq = &ui.NewRequest{LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start guest chrome on ther DUT: ", err)
	}

	// Pressing a character key resolves the issue of the keyboard settings disappearing,
	// but it may result in some key functions not performing as expected. The bug link is provided below.
	// https://partnerissuetracker.corp.google.com/u/0/issues/296981810
	// https://partnerissuetracker.corp.google.com/issues/255684001
	if err := utils.SinglePressKeyboard(ctx, utils.ChatKeyCodes["G"].KeyCodeBytes, 100*time.Millisecond); err != nil {
		s.Fatal("Failed to press 'g': ", err)
	}
	device := "/dev/input/by-id/" + kbdList[0]
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	kb := inputs.NewKeyboardServiceClient(cl.Conn)
	if err := utils.CheckTopRowkeys(ctx, s, device); err != nil {
		s.Fatal("Failed to check toprow keys: ", err)
	}
	if err := utils.CheckSpecialKeys(ctx, s, cl, appsSvc, uiautoSvc, kb, device); err != nil {
		s.Fatal("Failed to check Special keys: ", err)
	}
	if err := utils.CheckCharacterKeys(ctx, s, cl, appsSvc, uiautoSvc, kb, device); err != nil {
		s.Fatal("Failed to check character keys: ", err)
	}
}
