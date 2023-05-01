// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	pb "chromiumos/tast/services/cros/apps"
	inputspb "chromiumos/tast/services/cros/inputs"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NightLightViaDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test night light mode with dock and change color temperature from cooler to warmer",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.inputs.KeyboardService",
		},
	})
}
func NightLightViaDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
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
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	// Open IP power to supply docking power.
	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to open IP power: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

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

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	// GoBigSleepLint: Wait for external display to show the screen.
	testing.Sleep(ctx, 30*time.Second)

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Settings", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch setting: ", err)
	}

	settingsDeviceFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LINK}},
			{Value: &ui.NodeWith_Name{Name: "Device"}},
		},
	}

	settingsDisplayFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LINK}},
			{Value: &ui.NodeWith_Name{Name: "Displays"}},
		},
	}

	settingsNightLightToggleFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TOGGLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Night Light"}},
		},
	}

	settingsColorTemperatureContainerFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_GENERIC_CONTAINER}},
			{Value: &ui.NodeWith_Name{Name: "Color temperature"}},
		},
	}

	settingsColorTemperatureSliderFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_SLIDER}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsColorTemperatureContainerFinder}},
		},
	}

	notificationView := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "AshNotificationView"}},
		},
	}

	// Prevent peripherals connection notification to affect UI automation.
	if _, err := uiautoSvc.WaitUntilGone(ctx, &ui.WaitUntilGoneRequest{Finder: notificationView}); err != nil {
		s.Fatal("Failed to wait for notification view gone from context menu: ", err)
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: settingsDeviceFinder}); err != nil {
		s.Fatal("Failed to wait for Settings - Device from context menu: ", err)
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: settingsDeviceFinder}); err != nil {
		s.Fatal("Failed to click Settings - Device from context menu: ", err)
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: settingsDisplayFinder}); err != nil {
		s.Fatal("Failed to wait for Settings - Display from context menu: ", err)
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: settingsDisplayFinder}); err != nil {
		s.Fatal("Failed to click Settings - Display from context menu: ", err)
	}

	// Scroll down to make night light toggle visible.
	if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "Search+Down"}); err != nil {
		s.Fatal("Failed to type Search+Down: ", err)
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: settingsNightLightToggleFinder}); err != nil {
		s.Fatal("Failed to wait for night light toggle from context menu: ", err)
	}

	nightLightInfo, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: settingsNightLightToggleFinder})
	if err != nil {
		s.Fatal("Failed to get node info for night light toggle: ", err)
	}

	// Enable night light mode.
	if nightLightInfo.NodeInfo.Checked == ui.Checked_CHECKED_FALSE {
		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: settingsNightLightToggleFinder}); err != nil {
			s.Fatal("Failed to click night light toggle from context menu: ", err)
		}
	}

	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: settingsColorTemperatureSliderFinder}); err != nil {
		s.Fatal("Failed to wait for color temperature slider from context menu: ", err)
	}

	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: settingsColorTemperatureSliderFinder}); err != nil {
		s.Fatal("Failed to click color temperature slider from context menu: ", err)
	}

	extDisplayDefaultHV, err := utils.GetGamHotColdValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get color value from camera on the external display: ", err)
	}

	dutDefaultHCV, err := utils.GetGamHotColdValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get color value from camera on DUT monitor: ", err)
	}

	// Set color temperature to warmer.
	if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "Search+Right"}); err != nil {
		s.Fatal("Failed to type Search+Right: ", err)
	}

	sliderInfoWarmer, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: settingsColorTemperatureSliderFinder})
	if err != nil {
		s.Fatal("Failed to get node info for color temperature slider: ", err)
	}

	if sliderInfoWarmer.NodeInfo.Value != "100" {
		s.Fatalf("Failed to set color temperature value to 100; got %s, want 100", sliderInfoWarmer.NodeInfo.Value)
	}

	extDisplayWarmerHCV, err := utils.GetGamHotColdValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get color value from camera on the external display: ", err)
	}

	dutWarmerHCV, err := utils.GetGamHotColdValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get color value from camera on DUT monitor: ", err)
	}

	// Set color temperature to cooler.
	if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: "Search+Left"}); err != nil {
		s.Fatal("Failed to type Search+Left: ", err)
	}

	sliderInfoCooler, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: settingsColorTemperatureSliderFinder})
	if err != nil {
		s.Fatal("Failed to get node info for color temperature slider: ", err)
	}

	if sliderInfoCooler.NodeInfo.Value != "0" {
		s.Fatalf("Failed to set color temperature to cooler; got %s, want 0", sliderInfoCooler.NodeInfo.Value)
	}

	extDiplayCoolerHCV, err := utils.GetGamHotColdValue(ctx, s, extDispID)
	if err != nil {
		s.Fatal("Failed to get color value from camera on the external display: ", err)
	}

	dutCoolerHCV, err := utils.GetGamHotColdValue(ctx, s, utils.DUTMonitor)
	if err != nil {
		s.Fatal("Failed to get color value from camera on DUT monitor: ", err)
	}

	if extDiplayCoolerHCV > extDisplayDefaultHV || extDisplayDefaultHV > extDisplayWarmerHCV {
		s.Fatalf("Unexpect color value detected on external display; cooler: %d, default: %d, warmer: %d", extDiplayCoolerHCV, extDisplayDefaultHV, extDisplayWarmerHCV)
	}

	if dutCoolerHCV > dutDefaultHCV || dutDefaultHCV > dutWarmerHCV {
		s.Fatalf("Unexpect color value detected on DUT monitor; cooler: %d, default: %d, warmer: %d", dutCoolerHCV, dutDefaultHCV, dutWarmerHCV)
	}
}
