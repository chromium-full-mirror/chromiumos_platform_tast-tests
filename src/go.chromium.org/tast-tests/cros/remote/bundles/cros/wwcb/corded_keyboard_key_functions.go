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
		Func:         CordedKeyboardKeyFunctions,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the functionality of the top row function keys, special keys, and character keys on the corded keyboard",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.AutomationService", "tast.cros.apps.AppsService", "tast.cros.inputs.KeyboardService"},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "enableServoAndDisableTabletMode",
		}, {
			Name:    "tablet_mode",
			Fixture: "enableServoAndTabletMode",
		}},
	})
}

func CordedKeyboardKeyFunctions(ctx context.Context, s *testing.State) {
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
		s.Fatal("Failed to turn on fixture to connect the corded keyboard: ", err)
	}

	// Retrieve the kbd device event and verify its count.
	kbdList, err := utils.GetDeviceEventKbd(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to retrieve the devices event kbd: ", err)
	} else if len(kbdList) != 1 {
		s.Fatalf("Failed to proceed due to incorrect number of device event kbd, expect: 1, actual: %d", len(kbdList))
	}

	device := "/dev/input/by-id/" + kbdList[0]

	if err := utils.InitSimulator(ctx); err != nil {
		s.Fatal("Failed to initialize simulator: ", err)
	}
	defer utils.ReleaseSimulator(ctx)

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
