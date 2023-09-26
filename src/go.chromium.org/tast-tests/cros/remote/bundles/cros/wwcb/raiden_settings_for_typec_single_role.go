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
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RaidenSettingsForTypecSingleRole,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the correctness of switching power sources behavior",
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
func RaidenSettingsForTypecSingleRole(ctx context.Context, s *testing.State) {
	/*
		Testing TypeC chrome book with android phone:
		1- Open settings app.
		2- Switch to device/power page.
		3- Change power source to USB-C device.
		4- Check device battery status is charging.
		5- Close lid and resume.
		6- Check device battery status is charging.
		7- Turn off android phone charge connect device.
		8- Check device battery status is discharging.
		9- Close lid and resume.
		10- Check device battery status is discharging.
		11- Change power source to USB-C device.
		12- Check device battery status is charging.
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	isTablet := s.Param().(bool)
	usbTypeCID := s.RequiredVar("USBTypeCID")

	// Connect to the gRPC server on the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to the servo: ", err)
	}
	defer pxy.Close(cleanupCtx)
	cleanupCmd, err := utils.EnableDUTInTabletMode(ctx, dut, pxy, isTablet)
	if err != nil {
		s.Fatal("Failed to enable tablet or clamshell mode: ", err)
	}
	defer pxy.Servo().RunECCommand(cleanupCtx, cleanupCmd)

	// Cutting off the servo power supply, in case of affecting on verification of docking station power.
	if err := pxy.Servo().SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer pxy.Servo().SetPDRole(cleanupCtx, servo.PDRoleSrc)

	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	defer appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Settings"})

	powerSourceFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_COMBO_BOX_SELECT}},
			{Value: &ui.NodeWith_NameContaining{NameContaining: "source"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	usbcDeviceFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LIST_BOX_OPTION}},
			{Value: &ui.NodeWith_Name{Name: "USB-C device"}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	// Connect android.
	if err := utils.ControlFixture(ctx, usbTypeCID, "on"); err != nil {
		s.Fatal("Failed to connect android: ", err)
	}

	if err := utils.OpenPowerSettings(ctx, cs, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to open power settings: ", err)
	}
	s.Log("Change power source to USB-C device")
	if err := utils.ClickPowerSourceOption(ctx, uiautoSvc, powerSourceFinder, usbcDeviceFinder); err != nil {
		s.Fatal("Failed to change power source to USB-C device: ", err)
	}
	// Check power battery is charging or full.
	if err := utils.WaitUntilBatteryStatus(ctx, uiautoSvc, true); err != nil {
		s.Fatal("Failed to verify power status after change source to USB-C device: ", err)
	}

	s.Log("Close lid")
	if err := pxy.Servo().CloseLid(ctx); err != nil {
		s.Fatal("Failed to close lid: ", err)
	}

	if err := dut.WaitUnreachable(ctx); err != nil {
		s.Fatal("Failed to wait for unreachable: ", err)
	}

	if err := pxy.Servo().OpenLid(ctx); err != nil {
		s.Error("Failed to lid open: ", err)
	}
	cl, err = rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after resume lid: ", err)
	}
	// Start Chrome on the DUT.
	cs = ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome after resume lid: ", err)
	}
	appsSvc = pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc = ui.NewAutomationServiceClient(cl.Conn)

	if err := utils.OpenPowerSettings(ctx, cs, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to open power settings after resume lid: ", err)
	}
	// Check power battery is charging or full.
	if err := utils.WaitUntilBatteryStatus(ctx, uiautoSvc, true); err != nil {
		s.Fatal("Failed to verify power status after change source to USB-C device after resume lid: ", err)
	}
	s.Log("Turn off android charge connect device")
	if err := dut.Conn().CommandContext(ctx, "adb", "shell", "am", "start", "-a", "android.settings.BLUETOOTH_SETTINGS").Run(exec.DumpLogOnError); err != nil {
		s.Fatal("Failed to adb start usb settings: ", err)
	}

	if err := utils.ClickElement(ctx, dut, "USB", utils.FindElementByText); err != nil {
		s.Fatal("Failed to tap usb option: ", err)
	}
	if err := utils.ClickElement(ctx, dut, "android.widget.Switch", utils.FindElementByClass); err != nil {
		s.Fatal("Failed to tap charge connect device: ", err)
	}
	// Check power is discharging.
	if err := utils.WaitUntilBatteryStatus(ctx, uiautoSvc, false); err != nil {
		s.Fatal("Failed to verify power is discharging after change android charge state: ", err)
	}

	s.Log("Close lid again")
	if err := pxy.Servo().CloseLid(ctx); err != nil {
		s.Fatal("Failed to close lid after change android charge state: ", err)
	}

	if err := dut.WaitUnreachable(ctx); err != nil {
		s.Fatal("Failed to wait for unreachable after change android charge state: ", err)
	}

	if err := pxy.Servo().OpenLid(ctx); err != nil {
		s.Error("Failed to lid open after change android charge state: ", err)
	}

	cl, err = rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT after second resume lid: ", err)
	}
	// Start Chrome on the DUT.
	cs = ui.NewChromeServiceClient(cl.Conn)
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome after second resume lid: ", err)
	}
	appsSvc = pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc = ui.NewAutomationServiceClient(cl.Conn)

	if err := utils.OpenPowerSettings(ctx, cs, appsSvc, uiautoSvc); err != nil {
		s.Fatal("Failed to open power settings after second resume lid: ", err)
	}
	// Check power is discharging.
	if err := utils.WaitUntilBatteryStatus(ctx, uiautoSvc, false); err != nil {
		s.Fatal("Failed to verify power is discharging after second resume lid: ", err)
	}
	s.Log("Change power source to USB-C device")
	if err := utils.ClickPowerSourceOption(ctx, uiautoSvc, powerSourceFinder, usbcDeviceFinder); err != nil {
		s.Fatal("Failed to change power source to USB-C device after reconnect android: ", err)
	}
}
