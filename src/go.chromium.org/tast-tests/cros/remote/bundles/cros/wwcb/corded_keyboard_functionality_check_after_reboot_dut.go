// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CordedKeyboardFunctionalityCheckAfterRebootDUT,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the external keyboard functionality after reboot DUT",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_hid"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.inputs.KeyboardService",
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

func CordedKeyboardFunctionalityCheckAfterRebootDUT(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()
	USBID := s.RequiredVar("USBID")

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, USBID, "on"); err != nil {
		s.Fatal("Failed to control fixture to connect the USB keyboard emulator: ", err)
	}

	// Retrieve the kbd device event and verify its count.
	kbdList, err := utils.GetDeviceEventKbd(ctx, dut)
	if err != nil {
		s.Fatal("Failed to retrieve the keyboard emulator info: ", err)
	} else if len(kbdList) != 1 {
		s.Fatalf("Expect to get only one keyboard emulator, but got: %d", len(kbdList))
	}

	keyboardEmulator := "/dev/input/by-id/" + kbdList[0]
	testing.ContextLogf(ctx, "Found the USB keyboard emulator: %s", keyboardEmulator)

	s.Log("Start to reboot DUT")
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

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

	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)

	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(cleanupCtx)

	if err := utils.CheckTopRowkeys(ctx, s, keyboardEmulator); err != nil {
		s.Fatal("Failed to check top row keys: ", err)
	}

	if err := utils.CheckSpecialKeys(ctx, s, cl, appsSvc, uiautoSvc, keyboardSvc, keyboardEmulator); err != nil {
		s.Fatal("Failed to check special keys: ", err)
	}

	if err := utils.CheckCharacterKeys(ctx, s, cl, appsSvc, uiautoSvc, keyboardSvc, keyboardEmulator); err != nil {
		s.Fatal("Failed to check character keys: ", err)
	}
}
