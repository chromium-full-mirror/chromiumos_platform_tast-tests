// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           TurnOffWifi,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify WiFi networks should not to be listed after WiFi is turned off and should not be able to ping through the previously assigned IP",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wifiutil.FaillogServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
	})
}

// TurnOffWifi verifies WiFi networks should not to be listed after WiFi is turned off and
// should not be able to ping through the previously assigned IP.
func TurnOffWifi(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	// Only one ap will be connected and interacted during the test.
	var ap *wificell.APIface
	// Simulating the presence of multiple Wi-Fi networks within range.
	for i := 0; i <= 2; i++ {
		var err error
		ap, err = tf.DefaultOpenNetworkAP(ctx)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}

		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()
		defer tf.DeconfigAP(cleanupCtx, ap)
	}

	wifiSvc := tf.DUTWifiClient(wificell.DefaultDUT)
	// Ensure the WiFi is enabled to perform the test.
	if err := wifiSvc.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to set WiFi enable: ", err)
	}

	if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap); err != nil {
		s.Fatalf("Failed to connect to the WiFi %q: %v", ap.Config().SSID, err)
	}
	cleanupCtx = ctx
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	quickSettingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	if _, err := quickSettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to navigate to network detailed view within Quick Settings: ", err)
	}
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, "turn_off_wifi_ui_dump")

	// Turn off WiFi to verify the behavior under WiFi is turned off.
	if _, err := quickSettingsSvc.ToggleOption(ctx, &quicksettings.ToggleOptionRequest{
		ToggleButton: quicksettings.ToggleOptionRequest_Wifi,
		Enabled:      false,
	}); err != nil {
		s.Fatal("Failed to turn off WiFi: ", err)
	}
	defer wifiSvc.SetWifiEnabled(cleanupCtx, true)

	// WiFi networks should not be listed when WiFi is turned off.
	if resp, err := quickSettingsSvc.AvailableWifiNetworks(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to retrieve the available WiFi network from Quick Settings: ", err)
	} else if len(resp.GetSsids()) != 0 {
		s.Fatal("Failed to verify WiFi networks should not be listed, WiFi network listed")
	}

	// Verify that the wireless connectivity is NOT available by pinging the AP through the WiFi client interface.
	result, err := tf.PingFromSpecificDUT(ctx, wificell.DefaultDUT, ap.ServerIP().String())
	if err != nil {
		// Ignore the error since wireless connectivity should NOT be available at this point
		// and an error is expected.
		return
	}
	if result != nil && result.Sent != 0 {
		s.Fatal("Failed to verify wireless connectivity should not be available when WiFi is turned off")
	}
}
