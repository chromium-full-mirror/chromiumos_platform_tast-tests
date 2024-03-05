// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wwcb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelftestWebcam,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check whether the host's webcam can be mapped correctly",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "category"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.apps.AppsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wwcb.DisplayService"},
		Data: []string{"Capabilities.json"},
	})
}

func SelftestWebcam(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	category := s.RequiredVar("category")

	switch category {
	case "docking", "docking_daisychain", "monitor", "monitor_daisychain":
		break
	default:
		s.Fatalf("Failed to test %s category", category)
	}

	dut := s.DUT()

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to initialize the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Read Capabilities.json.
	capabilitiesMap, err := utils.ReadConfigFile(s)
	if err != nil {
		s.Fatal("Failed to read config file: ", err)
	}

	// Get the fixture ID of the display.
	downstream, ok := capabilitiesMap["Downstream"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to Downstream devices")
	}

	display, ok := downstream["Display"].(map[string]interface{})
	if !ok {
		s.Fatal("Failed to find displays")
	}
	capabilitiesDisplayInterface := map[string]interface{}{}
	for portName, portData := range display {
		portInfo, ok := portData.(map[string]interface{})
		if !ok {
			s.Fatalf("Failed to get the port data, port:%s", portName)
		}
		for key, data := range portInfo {
			if strings.Contains(key, "ExtDispID") {
				capabilitiesDisplayInterface[key] = data
			}
		}
	}
	var keys []string
	for key := range capabilitiesDisplayInterface {
		keys = append(keys, key)
	}

	// Start mapping.
	displaySvc := wwcb.NewDisplayServiceClient(cl.Conn)
	appsSvc := pb.NewAppsServiceClient(cl.Conn)
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	fs := dutfs.NewClient(cl.Conn)

	if err := utils.InitWebcam(ctx, s); err != nil {
		s.Fatal("Failed to initialize webcam: ", err)
	}

	switch category {
	case "docking", "docking_daisychain":
		// Open IP power to supply docking power.
		ipPowerPorts := []int{1}
		if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
			s.Fatal("Failed to open IP power: ", err)
		}
		defer utils.CloseIppower(cleanupCtx, ipPowerPorts)

		// Retrieve the ID of the docking fixture.
		capabilitiesUpstream := capabilitiesMap["Upstream"]
		if capabilitiesUpstream == nil {
			s.Fatal("Failed to find Upstream")
		}
		capabilitiesDockingID := capabilitiesUpstream.(map[string]interface{})["DockingID"]
		if capabilitiesDockingID == nil {
			s.Fatal("Failed to find DockingID")
		}

		// Turn on docking fixture.
		if err := utils.ControlFixture(ctx, capabilitiesDockingID.(string), "on"); err != nil {
			s.Fatalf("Failed to open fixture: %s", capabilitiesDockingID.(string))
		}
		defer utils.ControlFixture(ctx, capabilitiesDockingID.(string), "off")
	}

	for _, key := range keys {
		if err := utils.ControlFixture(ctx, capabilitiesDisplayInterface[key].(string), "on"); err != nil {
			s.Fatalf("Failed to connect to the external display, fixture ID:%s %v", capabilitiesDisplayInterface[key].(string), err)
		}
	}

	displayCount := len(keys) + 1
	if _, err := displaySvc.VerifyDisplayCount(ctx, &wwcb.QueryRequest{DisplayCount: int32(displayCount)}); err != nil {
		s.Fatal("Failed to verify display count: ", err)
	}

	displayIDs, err := displaySvc.GetDisplayIDs(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get display ID: ", err)
	} else if len(displayIDs.DisplayIds) < displayCount {
		s.Fatalf("Failed to get display ID; it must be greater than or equal to %d", displayCount)
	}

	// GoBigSleepLint: Wait for external display screen to show up.
	testing.Sleep(ctx, 30*time.Second)

	if err := utils.OpenRGBImageOnDisplays(ctx, fs, appsSvc, uiautoSvc, displaySvc); err != nil {
		s.Fatal("Failed to open RGB image on each display: ", err)
	}

	if err := utils.PairWebcamToDisplay(ctx, s, displayIDs.DisplayIds); err != nil {
		s.Fatal("Failed to pair webcam to display: ", err)
	}

	// Close the Gallery app.
	if _, err := appsSvc.CloseApp(ctx, &pb.CloseAppRequest{AppName: "Gallery", TimeoutSecs: 60}); err != nil {
		s.Fatal("Failed to close Gallery app: ", err)
	}

	for _, key := range keys {
		if err := utils.ControlFixture(ctx, capabilitiesDisplayInterface[key].(string), "off"); err != nil {
			s.Fatalf("Failed to disconnect to the external display, fixture ID:%s %v", capabilitiesDisplayInterface[key].(string), err)
		}
	}
}
