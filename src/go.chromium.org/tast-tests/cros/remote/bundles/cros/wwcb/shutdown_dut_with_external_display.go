// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShutdownDUTWithExternalDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Shutdown DUT then check both screens on DUT & external display to become dark by camera connecting to the host",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.wwcb.DisplayService"},
	})
}

func ShutdownDUTWithExternalDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	// What if the script is testing for dock test case, it will need to power on docking station.
	// Then do the mapping the camera to display fixture with docking station connected to DUT.
	extDispIDArray := []string{extDispID}
	if dockingID, ok := s.Var("DockingID"); ok {
		ippowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
			s.Fatal("Failed to power on the docking station: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ippowerPorts)

		if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
			s.Fatal("Failed to connect to the docking station in the state of mapping the display fixture to camera: ", err)
		}

		if err := utils.MappingDisplayFixtureToCamera(ctx, s, extDispIDArray); err != nil {
			s.Fatal("Failed to do mapping display fixture to camera: ", err)
		}

		if err := utils.ControlFixture(ctx, dockingID, "off"); err != nil {
			s.Fatal("Failed to disconnect to the docking station in the state of mapping the display fixture to camera: ", err)
		}
	} else {
		if err := utils.MappingDisplayFixtureToCamera(ctx, s, extDispIDArray); err != nil {
			s.Fatal("Failed to do mapping display fixture to camera: ", err)
		}
	}

	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if dockingID, ok := s.Var("DockingID"); ok {
		if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
			s.Fatal("Failed to connect to the docking station: ", err)
		}
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

	// Shutdown DUT.
	powerOffCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Failed to execute shutdown command: ", err)
	}
	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed to wait for unreachable: ", err)
	}
	defer utils.PowerOnDUT(ctx, pxy, dut)

	extDispShutdownLight, err := utils.GetGamLightingValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get the external display light during shutdown: ", err)
	}

	dutShutdownLight, err := utils.GetGamLightingValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get the DUT light during shutdown: ", err)
	}

	// Check external display & DUT screen to become dark by camera connecting to host.
	if extDispShutdownLight >= extDispDefaultLight {
		s.Fatalf("Expect the external display light during shutdown is equal or lower than default; shutdown light: %d, default light: %d", extDispShutdownLight, extDispDefaultLight)
	}

	if dutShutdownLight >= dutDefaultLight {
		s.Fatalf("Expect the DUT light during shutdown is equal or lower than default; shutdown light: %d, default light: %d", dutShutdownLight, dutDefaultLight)
	}
}
