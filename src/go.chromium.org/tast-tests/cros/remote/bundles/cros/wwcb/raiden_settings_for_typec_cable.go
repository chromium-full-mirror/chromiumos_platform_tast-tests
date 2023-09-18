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
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RaidenSettingsForTypecCable,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the power source does not display a list when using a sink device",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBTypeCID"},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}},
		ServiceDeps: []string{"tast.cros.browser.ChromeService", "tast.cros.apps.AppsService", "tast.cros.ui.AutomationService"},
	})
}
func RaidenSettingsForTypecCable(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	usbTypeCID := s.RequiredVar("USBTypeCID")
	isTablet := s.Param().(bool)
	launchAppTimeout := 60

	// Connect to the gRPC server on the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Cut off the servo power supply, in case of affecting on verification of docking station power.
	if err := pxy.Servo().SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer pxy.Servo().SetPDRole(cleanupCtx, servo.PDRoleSrc)

	if isTablet {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	} else {
		testing.ContextLog(ctx, "Enable clamshell mode")
	}

	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
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

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, usbTypeCID, "on"); err != nil {
		s.Fatal("Failed to turn on typec fixture: ", err)
	}

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Settings", TimeoutSecs: int32(launchAppTimeout)}); err != nil {
		s.Fatal("Failed to launch settings app: ", err)
	}

	deviceFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Device"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	powerFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "Power"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	powerSourceFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_COMBO_BOX_SELECT}},
			{Value: &ui.NodeWith_Name{Name: "Power source"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: deviceFinder}); err != nil {
		s.Fatal("Failed to wait device button: ", err)
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: deviceFinder}); err != nil {
		s.Fatal("Failed to click device button: ", err)
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: powerFinder}); err != nil {
		s.Fatal("Failed to wait power button: ", err)
	}
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: powerFinder}); err != nil {
		s.Fatal("Failed to click power button: ", err)
	}

	// Power source list should not show up.
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: powerSourceFinder}); err == nil {
		s.Fatal("Failed to verify power source not show up list")
	}

	// Check power is discharging.
	if err := utils.VerifyPowerStatus(ctx, dut, false); err != nil {
		s.Fatal("Failed to verify power status: ", err)
	}
}
