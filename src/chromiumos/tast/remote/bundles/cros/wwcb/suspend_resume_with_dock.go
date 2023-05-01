// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook
package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/powercontrol"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendResumeWithDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Suspend/resume DUT then check screen light on DUT & external display by camera connecting to the host",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService", "tast.cros.wwcb.DisplayService", "tast.cros.inputs.KeyboardService"},
	})
}

func SuspendResumeWithDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

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

	// Open IP power to supply docking power.
	ipPowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
		s.Fatal("Failed to power on docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	extDispIDArray := []string{extDispID}
	if err := utils.MappingWithDockFixture(ctx, s, extDispIDArray, dockingID); err != nil {
		s.Fatal("Failed to mapping display fixture to camera: ", err)
	}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}

	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	extDispDefaultLight, err := utils.GetGamLightingValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get the external display light from camera: ", err)
	}

	dutDefaultLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get the DUT light from camera: ", err)
	}

	if err := powercontrol.PerformPowerdbusSuspend(ctx, s.DUT(), pxy); err != nil {
		s.Fatal("Failed to perform powerdbus suspend: ", err)
	}

	firmwareHelper := &firmware.Helper{Servo: pxy.Servo()}
	if err := powercontrol.WaitForSuspendState(ctx, firmwareHelper); err != nil {
		s.Fatal("Failed to wait for DUT suspend state: ", err)
	}

	extDispSuspendLight, err := utils.GetGamLightingValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get the external display light in suspend mode: ", err)
	}

	dutSuspendLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get the DUT light in suspend mode: : ", err)
	}

	// Check external display & DUT screen to become dark by camera connecting to the host.
	if extDispSuspendLight >= extDispDefaultLight {
		s.Fatalf("Expect the external display light in suspend mode is equal to lower than default; suspended light: %d, default light: %d", extDispSuspendLight, extDispDefaultLight)
	}

	if dutSuspendLight >= dutDefaultLight {
		s.Fatalf("Expect the DUT light in suspend mode is equal to lower than default; suspended light: %d, default light: %d", dutSuspendLight, dutDefaultLight)
	}

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to wake DUT: ", err)
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	extDispWakenLight, err := utils.GetGamLightingValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get the external display light while DUT wake up: ", err)
	}

	dutWakenLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get the DUT light while DUT wake up: ", err)
	}

	// Check external display & DUT screen to turn on by camera connecting to host.
	if extDispSuspendLight >= extDispWakenLight {
		s.Fatalf("Expect the external display light in suspend mode is equal or lower than waken; suspended light: %d, waken light: %d", extDispSuspendLight, extDispWakenLight)
	}

	if dutSuspendLight >= dutWakenLight {
		s.Fatalf("Expect the DUT light in suspend mode is equal or lower than waken; suspended light: %d, waken light: %d", dutSuspendLight, dutWakenLight)
	}
}
