// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

var (
	// settingsWindowFinder is the finder of settings window.
	settingsWindowFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_WINDOW}},
			{Value: &ui.NodeWith_HasClass{HasClass: "BrowserFrame"}},
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Settings"}},
		},
	}

	// settingsDeviceLinkFinder is the finder of the device link.
	settingsDeviceLinkFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LINK}},
			{Value: &ui.NodeWith_Name{Name: "Device"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsWindowFinder}},
		},
	}

	// settingsDisplayLinkFinder is the finder of the displays link.
	settingsDisplayLinkFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_LINK}},
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Display"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsWindowFinder}},
		},
	}

	// settingsDisplayFinder is the finder of the night light toggle button.
	settingsNightLightToggleFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_TOGGLE_BUTTON}},
			{Value: &ui.NodeWith_Name{Name: "Night Light"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsWindowFinder}},
		},
	}

	// settingsColorTemperatureContainerFinder is the finder of the color temperature container.
	settingsColorTemperatureContainerFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_GENERIC_CONTAINER}},
			{Value: &ui.NodeWith_Name{Name: "Color temperature"}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsWindowFinder}},
		},
	}

	// settingsColorTemperatureSliderFinder is the finder of the color temperature slider.
	settingsColorTemperatureSliderFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_SLIDER}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: settingsColorTemperatureContainerFinder}},
		},
	}

	// notificationViewFinder is the finder of the pop-up notification view.
	notificationViewFinder = &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "AshNotificationView"}},
		},
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NightLightViaDock,
		Desc:         "Test night light mode with dock and change color temperature from cooler to warmer",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "newTestItem"},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcb.dock",
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.ui.ScreenRecorderService",
		},
		Params: []testing.Param{
			{
				Name:      "full",
				ExtraAttr: []string{"pasit_full"},
			}},
	})
}
func NightLightViaDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Start screen recording, defer the stop and saving of said recording.
	screenRecorder := ui.NewScreenRecorderServiceClient(cl.Conn)
	utils.StartRecording(ctx, s, screenRecorder)
	defer utils.StopAndSaveScreenRecording(cleanupCtx, s, screenRecorder)

	fs := dutfs.NewClient(cl.Conn)
	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	keyboardSvc := inputspb.NewKeyboardServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	tf := s.FixtValue().(*topology.TestFixture)
	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}
	}

	// Connect the external display & Dock.
	if _, _, err := tf.Helper.ActivateDeviceByTypeVia(ctx, topology.DeviceTypeMonitor, topology.DeviceTypeDockingStation); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: 2}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	displayIDs, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	} else if len(displayIDs.DisplayIds) < 2 {
		s.Fatal("Failed to get display ID; it must be greater than or equal to 2")
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	if err := utils.OpenRGBImageOnDisplays(ctx, fs, appsSvc, uiautoSvc, displaySvc); err != nil {
		s.Fatal("Failed to open RGB image on each display: ", err)
	}

	parings, err := tf.CameraHelper.PairWebcamToDisplay(ctx, s.OutDir(), displayIDs.DisplayIds)
	if err != nil {
		s.Fatal("Failed to pair webcam to display: ", err)
	}

	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Gallery app: ", err)
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			uiTreeResponse, err := uiautoSvc.GetUITree(ctx, &ui.GetUITreeRequest{})
			if err != nil {
				s.Log("Unable to get UI tree: ", err)
			}

			if err := os.WriteFile(filepath.Join(s.OutDir(), "ui_tree.txt"), []byte(uiTreeResponse.UiTree), 0644); err != nil {
				s.Log("Unable to save UI tree on the host: ", err)
			}
		}
	}(cleanupCtx)

	// Prevent peripherals connection notification to affect UI automation.
	if _, err := uiautoSvc.WaitUntilGone(ctx, &ui.WaitUntilGoneRequest{Finder: notificationViewFinder}); err != nil {
		s.Fatal("Failed to wait for notification view gone from context menu: ", err)
	}

	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "Settings", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to launch setting: ", err)
	}

	dutHCVs := make(map[string]int)
	extDispHCVs := make(map[string]int)

	nightLightOff := "off"

	extDispCamera := parings[displayIDs.DisplayIds[1]]
	extDispHCVs[nightLightOff], err = tf.CameraHelper.GAMHotColdValue(ctx, s.OutDir(), extDispCamera)

	if err != nil {
		s.Fatal("Failed to get the external display HCV during night light is off: ", err)
	}
	s.Logf("External display HCV during night light is off: %d", extDispHCVs[nightLightOff])

	dutDispCamera := parings[displayIDs.DisplayIds[0]]
	dutHCVs[nightLightOff], err = tf.CameraHelper.GAMHotColdValue(ctx, s.OutDir(), dutDispCamera)
	if err != nil {
		s.Fatal("Failed to get the DUT HCV during night light is off: ", err)
	}
	s.Logf("DUT HCV during night light is off: %d", dutHCVs[nightLightOff])

	// Enable night light.
	for _, finder := range []*ui.Finder{
		settingsDeviceLinkFinder,
		settingsDisplayLinkFinder,
		settingsNightLightToggleFinder,
		settingsColorTemperatureSliderFinder,
	} {
		if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: finder}); err != nil {
			s.Fatalf("Failed to wait for %q from context menu: %v", finder, err)
		}

		if _, err := uiautoSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: finder}); err != nil {
			s.Fatalf("Failed to focus on %q from context menu: %v", finder, err)
		}

		if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: finder}); err != nil {
			s.Fatalf("Failed to click %q from context menu: %v", finder, err)
		}
	}

	nightLightCooler := "cooler"
	nightLightWarmer := "warmer"

	// Adjust color temperature.
	for _, test := range []struct {
		name                  string
		hotkey                string
		colorTemperatureValue string
	}{
		{nightLightWarmer, "Search+Right", "100"},
		{nightLightCooler, "Search+Left", "0"},
	} {
		if _, err := keyboardSvc.Accel(ctx, &inputspb.AccelRequest{Key: test.hotkey}); err != nil {
			s.Fatalf("Failed to type %s: %v", test.hotkey, err)
		}

		clrTmpSilderInfo, err := uiautoSvc.Info(ctx, &ui.InfoRequest{Finder: settingsColorTemperatureSliderFinder})
		if err != nil {
			s.Fatal("Failed to get node info of color temperature slider: ", err)
		}

		if clrTmpSilderInfo.NodeInfo.Value != test.colorTemperatureValue {
			s.Fatalf("Failed to set color temperature; got %s, want %s", clrTmpSilderInfo.NodeInfo.Value, test.colorTemperatureValue)
		}

		extDispHCVs[test.name], err = tf.CameraHelper.GAMHotColdValue(ctx, s.OutDir(), extDispCamera)
		if err != nil {
			s.Fatalf("Failed to get the external display HCV during night light is %s: %v", test.name, err)
		}
		s.Logf("External display HCV during night light is %s: %d", test.name, extDispHCVs[test.name])

		dutHCVs[test.name], err = tf.CameraHelper.GAMHotColdValue(ctx, s.OutDir(), dutDispCamera)
		if err != nil {
			s.Fatalf("Failed to get the DUT HCV during night light is %s: %v", test.name, err)
		}
		s.Logf("DUT HCV during night light is %s: %d", test.name, dutHCVs[test.name])
	}

	if extDispHCVs[nightLightOff] > extDispHCVs[nightLightWarmer] || extDispHCVs[nightLightCooler] > extDispHCVs[nightLightWarmer] {
		s.Fatalf("Unexpect the external display HCV; off: %d, warmer: %d, cooler: %d", extDispHCVs[nightLightOff], extDispHCVs[nightLightWarmer], extDispHCVs[nightLightCooler])
	}

	if dutHCVs[nightLightOff] > dutHCVs[nightLightWarmer] || dutHCVs[nightLightCooler] > dutHCVs[nightLightWarmer] {
		s.Fatalf("Unexpect the DUT HCV; off: %d, warmer: %d, cooler: %d", dutHCVs[nightLightOff], dutHCVs[nightLightWarmer], dutHCVs[nightLightCooler])
	}
}
