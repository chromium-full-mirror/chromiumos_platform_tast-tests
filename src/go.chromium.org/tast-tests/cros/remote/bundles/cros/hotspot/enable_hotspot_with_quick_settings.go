// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hotspot

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hotspot/hotspotutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableHotspotWithQuickSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that hotspot can be turned on and off with quick settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"jiajunz@google.com",
		},
		BugComponent: "b:1281224", // ChromeOS > Software > System Services > Connectivity > Hotspot
		Attr:         []string{"group:wificell_cross_device", "wificell_cross_device_sap", "wificell_cross_device_unstable"},
		ServiceDeps: []string{
			wificell.BrowserChromeServiceName,
			wificell.OsSettingsServiceName,
			wificell.QuickSettingsServiceName,
			wificell.ChromeUIServiceName,
			wificell.AutomationServiceName,
			wificell.CellularServiceName,
			wificell.ShillServiceName,
		},
		HardwareDeps: hwdep.D(hwdep.WifiSAP(), hwdep.Cellular(), hwdep.Model("crota")),
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesSelfManagedAP | wificell.TFFeaturesCellular),
		Timeout:      7 * time.Minute,
	})
}

// EnableHotspotWithQuickSettings tests that Hotspot can be turned on and off via quick settings by following steps:
// 1- connect to the cellular network on the main DUT
// 2- Go to hotspot subpage in Settings and toggle hotspot on and off in order to have hotspot pod show in quick settings.
// 3- Enable hotspot by navigating to the hotspot detailed view in Quick Settings.
// 4- Configures the companion DUT as a STA and connect to hotspot.
// 5- Toggle off hotspot from hotspot detailed view.
func EnableHotspotWithQuickSettings(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if _, err := cr.New(ctx, &ui.NewRequest{
		EnableFeatures: []string{wificell.ChromeFeatureHotspot},
	}); err != nil {
		s.Fatal("Failed to start Chrome with hotspot flag enabled: ", err)
	}

	cleanup, err := tf.SetupAndConnectCellular(ctx, wificell.DefaultDUT)
	if cleanup != nil {
		defer cleanup(cleanupCtx)
	}
	if err != nil {
		s.Fatal("Failed to setup cellular client and connect to cellular network: ", err)
	}

	ossettingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	defer ossettingsSvc.Close(cleanupCtx, &emptypb.Empty{})

	quicksettingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	s.AttachErrorHandlers(
		func(errMsg string) {
			hotspotutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, "ui_dump_error")
		},
		func(errMsg string) {
			hotspotutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, "ui_dump_fatal")
		},
	)

	resp, err := quicksettingsSvc.IsHotspotTileShown(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to check hotspot tile in Quick Settings: ", err)
	}
	if resp.GetIsHotspotTileShown() {
		s.Fatal("Hotspot tile should not be shown in Quick Settings")
	}

	// Turn on hotspot once to have it show in quick settings
	if _, err := ossettingsSvc.OpenHotspotDetailPage(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to launch hotspot subpage: ", err)
	}
	if _, err := ossettingsSvc.ToggleHotspot(ctx, &ossettings.ToggleHotspotRequest{Enabled: true}); err != nil {
		s.Fatal("Failed to toggle on hotspot: ", err)
	}
	if _, err := ossettingsSvc.ToggleHotspot(ctx, &ossettings.ToggleHotspotRequest{Enabled: false}); err != nil {
		s.Fatal("Failed to toggle off hotspot: ", err)
	}

	uiauto := ui.NewAutomationServiceClient(rpcClient.Conn)
	// Navigate to hotspot detailed view will also trigger enable hotspot.
	if _, err := quicksettingsSvc.NavigateToHotspotDetailedView(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to open hotspot detailed page: ", err)
	}
	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: hotspotutil.NoDeviceConnectedMessage}); err != nil {
		s.Fatal("Failed to verify no devices connected message in hotspot detailed page: ", err)
	}

	disconnectDUT, err := tf.ConnectCompanionDUTToHotspot(ctx, wificell.PeerDUT1, wificell.DefaultDUT)
	if err != nil {
		s.Fatal("Failed to connect companion DUT to hotspot: ", err)
	}
	defer disconnectDUT(cleanupCtx)

	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: hotspotutil.OneDeviceConnectedMessage}); err != nil {
		s.Fatal("Failed to verify one device connected message in hotspot detailed page: ", err)
	}
	if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: hotspotutil.HotspotSwitch}); err != nil {
		s.Fatal("Failed to toggle off hotspot in hotspot detailed page: ", err)
	}
	if _, err := uiauto.EnsureGone(ctx, &ui.EnsureGoneRequest{Finder: hotspotutil.OneDeviceConnectedMessage}); err != nil {
		s.Fatal("Failed to verify hotspot is turned off in detailed page: ", err)
	}
}
