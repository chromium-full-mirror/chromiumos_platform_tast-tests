// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hotspot

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hotspot/hotspotutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	networkPage = iota
	hotspotSubpage
)

type enableHotspotTestParam struct {
	enableWifi bool
	launchPage int
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
			wificell.AutomationServiceName,
			wificell.CellularServiceName,
			wificell.ShillServiceName,
		},
		HardwareDeps: hwdep.D(hwdep.WifiSAP(), hwdep.Cellular(), hwdep.Model("crota")),
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCompanionDUT | wificell.TFFeaturesCellular),
		Timeout:      7 * time.Minute,
		Params: []testing.Param{
			{
				Name: "with_wifi_enabled_from_network_page",
				Val: enableHotspotTestParam{
					enableWifi: true,
					launchPage: networkPage,
				},
			},
			{
				Name: "with_wifi_enabled_from_hotspot_subpage",
				Val: enableHotspotTestParam{
					enableWifi: true,
					launchPage: hotspotSubpage,
				},
			},
			{
				Name: "with_wifi_disabled_from_network_page",
				Val: enableHotspotTestParam{
					enableWifi: false,
					launchPage: networkPage,
				},
			},
			{
				Name: "with_wifi_disabled_from_hotspot_subpage",
				Val: enableHotspotTestParam{
					enableWifi: false,
					launchPage: hotspotSubpage,
				},
			},
		},
	})
}

// EnableHotspotWithOSSettings tests that Hotspot can be turned on and off via ossettings.
func EnableHotspotWithOSSettings(ctx context.Context, s *testing.State) {
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

	if testOpts.launchPage == networkPage {
		if _, err := ossettingsSvc.LaunchAtNetwork(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to launch OS-Settings at Network page: ", err)
		}
	} else if testOpts.launchPage == hotspotSubpage {
		if _, err := ossettingsSvc.OpenHotspotDetailPage(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to launch hotspot subpage: ", err)
		}
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

	tetheringConfig, err := wifiClient.GetTetheringConfig(ctx)
	if err != nil {
		s.Fatal("Failed to get hotspot config: ", err)
	}
	s.Logf("Using hotspot ssid: %s", tetheringConfig.Ssid)

	if tetheringConfig.Security != shillconst.SecurityWPA2 {
		s.Fatalf("Invalid tethering security: %s", tetheringConfig.Security)
	}

	fac := wpa.NewConfigFactory(
		tetheringConfig.Passphrase, wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP),
	)

	secConf, err := fac.Gen()
	if err != nil {
		s.Fatal("Failed to generate security config: ", err)
	}

	cdIdx := wificell.DutIdx(wificell.PeerDUT)
	_, err = tf.ConnectWifiFromDUT(ctx, cdIdx, tetheringConfig.Ssid, dutcfg.ConnSecurity(secConf))
	if err != nil {
		s.Fatal("Failed to connect to the hotspot, err: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DisconnectDUTFromWifi(ctx, cdIdx); err != nil {
			s.Error("Failed to disconnect from hotspot, err: ", err)
		}
	}(cleanupCtx)
	s.Log("Downstream DUT connected to the hotspot")

	// Navigate to hotspot detail page if needed and verify client count.
	if testOpts.launchPage == networkPage {
		if _, err := ossettingsSvc.OpenHotspotDetailPage(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to open hotspot detail page: ", err)
		}
	}

	uiauto := ui.NewAutomationServiceClient(rpcClient.Conn)
	clientCountNode := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Name{Name: "1"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
		},
	}
	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: clientCountNode}); err != nil {
		s.Fatal("Failed to verify client count update to 1 in hotspot subpage: ", err)
	}

	// Navigate back to Network page and toggle off hotspot
	if testOpts.launchPage == networkPage {
		if _, err := ossettingsSvc.LaunchAtNetwork(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to launch OS-Settings at Network page: ", err)
		}
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
