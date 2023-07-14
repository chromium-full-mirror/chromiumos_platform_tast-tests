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

type enableHotspotTestParam struct {
	enableWifi bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableHotspotWithOSSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that hotspot can be turned on and off with os settings",
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
			wificell.CellularServiceName,
			wificell.ShillServiceName,
		},
		HardwareDeps: hwdep.D(hwdep.WifiSAP(), hwdep.Cellular(), hwdep.Model("crota")),
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixtWithCellular",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "with_wifi_enabled",
				Val: enableHotspotTestParam{
					enableWifi: true,
				},
			},
			{
				Name: "with_wifi_disabled",
				Val: enableHotspotTestParam{
					enableWifi: false,
				},
			},
		},
	})
}

// EnableHotspotWithOSSettings tests that Hotspot can be turned on and off via ossettings.
func EnableHotspotWithOSSettings(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if _, err := cr.New(ctx, &ui.NewRequest{
		EnableFeatures: []string{wificell.ChromeFeatureHotspot},
	}); err != nil {
		s.Fatal("Failed to start Chrome with hotspot flag enabled: ", err)
	}

	if _, err := tf.DUTCellularClient(wificell.DefaultDUT).SetUp(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to initialize cellular shill service on DUT: ", err)
	}
	s.Log("Successfully initialized cellular device on DUT")
	defer tf.DUTCellularClient(wificell.DefaultDUT).TearDown(cleanupCtx, &emptypb.Empty{})

	if _, err := tf.DUTCellularClient(wificell.DefaultDUT).Connect(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to connect to cellular network on DUT: ", err)
	}
	s.Log("Successfully connected to the cellular network")

	testOpts := s.Param().(enableHotspotTestParam)

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, testOpts.enableWifi); err != nil {
		s.Fatal("Failed to initialize Wi-Fi state: ", err)
	}
	// Leaving the test with Wi-Fi enabled since Wi-Fi being enabled is the default "good" state for group:wificell.
	defer wifiClient.SetWifiEnabled(cleanupCtx, true)

	ossettingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	defer ossettingsSvc.Close(cleanupCtx, &emptypb.Empty{})

	quicksettingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	resp, err := quicksettingsSvc.IsHotspotTileShown(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to check hotspot tile in Quick Settings: ", err)
	}
	if resp.GetIsHotspotTileShown() {
		s.Fatal("Hotspot tile should not be shown in Quick Settings")
	}

	if _, err := ossettingsSvc.LaunchAtNetwork(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to launch OS-Settings at Network page: ", err)
	}
	defer hotspotutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, "ui_dump")

	if _, err := ossettingsSvc.ToggleHotspot(ctx, &ossettings.ToggleHotspotRequest{Enabled: true}); err != nil {
		s.Fatal("Failed to toggle on hotspot: ", err)
	}

	enabled, err := wifiClient.GetWifiEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi enabled state: ", err)
	}
	if enabled {
		s.Fatal("WiFi should be turned off after turning on hotspot")
	}

	if _, err := ossettingsSvc.ToggleHotspot(ctx, &ossettings.ToggleHotspotRequest{Enabled: false}); err != nil {
		s.Fatal("Failed to toggle off hotspot: ", err)
	}

	// Verify hotspot should be shown in Quick settings
	resp, err = quicksettingsSvc.IsHotspotTileShown(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to check hotspot tile in Quick Settings: ", err)
	}
	if !resp.GetIsHotspotTileShown() {
		s.Fatal("Hotspot tile should be shown in Quick Settings")
	}

	// TODO(jiajunz@): verify WiFi status is restored.
}
