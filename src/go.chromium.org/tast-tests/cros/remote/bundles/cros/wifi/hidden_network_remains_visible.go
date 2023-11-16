// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/network/protoutil"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HiddenNetworkRemainsVisible,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that hidden networks are visible after logging out/logging in and rebooting",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wifiutil.FaillogServiceName,
			"tast.cros.ui.AutomationService",
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
		},
		SoftwareDeps: []string{"chrome", "reboot"},
		// TODO(b/284522381, b/189972561): Remove the restrictions once the issue is resolved.
		HardwareDeps: hwdep.D(hwdep.SkipOnWifiDevice(wlan.QualcommWCN3990, wlan.MediaTekMT7921PCIE)),
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      7 * time.Minute, // It could take up to 4 minutes to reboot the DUT.
	})
}

// HiddenNetworkRemainsVisible verifies that hidden networks are visible after logging out/logging in and rebooting.
func HiddenNetworkRemainsVisible(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	opts := []hostapd.Option{
		hostapd.Channel(1),
		hostapd.SSID(hostapd.RandomSSID("Hidden_WiFi_")),
		hostapd.Mode(hostapd.Mode80211g),
		hostapd.Hidden(),
	}

	hiddenAp, err := tf.ConfigureAP(ctx, opts, nil)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer tf.DeconfigAP(ctx, hiddenAp)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, hiddenAp)
	defer cancel()

	anotherAP, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer tf.DeconfigAP(ctx, anotherAP)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, anotherAP)
	defer cancel()

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	startChromeReq := &ui.NewRequest{}

	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)
	// Join a hidden network at logged-in state as preparation of upcoming logout test.
	// Also, test if the hidden network remains visible after the network connection has switched to another network in this logged-in session.
	// Isolate the steps to leverage `defer` pattern.
	func(ctx context.Context) {
		cleanupCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
		if _, err := crSvc.New(ctx, startChromeReq); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
		if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
			Ssid:     hiddenAp.Config().SSID,
			Security: &wifi.JoinWifiRequest_None{},
		}); err != nil {
			s.Fatal("Failed to join wifi: ", err)
		}

		// WiFi networks will only become hidden by default if they are configured during OOBE or when logged out
		// and the network was not found in the list of WiFi networks found via scan. Explicitly mark this network as hidden. Explicitly mark this network as hidden.
		if err := toggleHiddenNetworkOn(ctx, rpcClient.Conn, hiddenAp.Config().SSID); err != nil {
			s.Fatal("Failed to turn on 'Hidden network' toggle button: ", err)
		}

		// Connect to another network to verify if the added hidden network
		// remains visible after the connection switched.
		if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, anotherAP); err != nil {
			s.Fatal("Failed to connect to another AP: ", err)
		}

		if err := verifyHiddenNetworkVisible(ctx, rpcClient.Conn, hiddenAp.Config().SSID); err != nil {
			s.Fatal("Failed to verify hidden WiFi is still visible: ", err)
		}
	}(ctx)

	// Logs the DUT out (via starting another Chrome session) to verify if the
	// added hidden network remains visible after re-login.
	if err := reLoginAndVerifyHiddenNetworkVisible(ctx, rpcClient.Conn, startChromeReq, hiddenAp.Config().SSID); err != nil {
		s.Fatal("Failed to verify hidden WiFi is still visible: ", err)
	}

	// Reboot the DUT to verify if the added hidden network remains visible after reboot.
	if err := tf.RebootDUT(ctx, wificell.DefaultDUT); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	rpcClient = tf.DUTRPC(wificell.DefaultDUT)
	if err := reLoginAndVerifyHiddenNetworkVisible(ctx, rpcClient.Conn, startChromeReq, hiddenAp.Config().SSID); err != nil {
		s.Fatal("Failed to verify hidden WiFi is still visible: ", err)
	}
}

func reLoginAndVerifyHiddenNetworkVisible(ctx context.Context, conn *grpc.ClientConn, startChromeReq *ui.NewRequest, ssid string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	startChromeReq.KeepState = true
	crSvc := ui.NewChromeServiceClient(conn)
	if _, err := crSvc.New(ctx, startChromeReq); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	return verifyHiddenNetworkVisible(ctx, conn, ssid)
}

// verifyHiddenNetworkVisible verifies the hidden network is visible on
// both Wi-Fi page and Known Networks and the network is a private network.
func verifyHiddenNetworkVisible(ctx context.Context, conn *grpc.ClientConn, ssid string) error {
	wifiSvc := wifi.NewWifiServiceClient(conn)
	if _, err := wifiSvc.WifiPageControl(ctx, &wifi.WifiPageControlRequest{
		Ssid:    ssid,
		Control: wifi.WifiPageControlRequest_WaitUntilExist,
	}); err != nil {
		return errors.Wrap(err, "failed to find the network on WiFi page")
	}

	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
	}); err != nil {
		return errors.Wrap(err, "failed to find the hidden network in the Known Networks")
	}

	shillService := wifi.NewShillServiceClient(conn)
	shillVal, err := protoutil.EncodeToShillValMap(map[string]interface{}{
		shillconst.ServicePropertyType:           shillconst.TypeWifi,
		shillconst.ServicePropertyWiFiHexSSID:    strings.ToUpper(hex.EncodeToString([]byte(ssid))),
		shillconst.ServicePropertyWiFiHiddenSSID: true,
	})
	if err != nil {
		return errors.Wrap(err, "failed to encode shill property")
	}

	path, err := shillService.GetServicePath(ctx, &wifi.ServicePathRequest{Props: shillVal})
	if err != nil {
		return errors.Wrap(err, "failed to get service path")
	}

	if _, err = shillService.QueryService(ctx, &wifi.QueryServiceRequest{Path: path.ServicePath}); err != nil {
		return errors.Wrap(err, "failed to find the target service")
	}
	return nil
}

func toggleHiddenNetworkOn(ctx context.Context, conn *grpc.ClientConn, ssid string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	osSettingsSvc := ossettings.NewOsSettingsServiceClient(conn)
	if _, err := osSettingsSvc.OpenNetworkDetailPage(ctx, &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: ssid,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}); err != nil {
		return errors.Wrap(err, "failed to open network detail page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			faillog := ui.NewChromeUIServiceClient(conn)
			faillog.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{
				FilePrefix: "network_details_page",
			})
		}
		osSettingsSvc.Close(ctx, &emptypb.Empty{})
	}(cleanupCtx)

	uiSvc := ui.NewAutomationServiceClient(conn)
	// The "Hidden network" option is located within the expandable "Network" section.
	if _, err := uiSvc.LeftClick(ctx, &ui.LeftClickRequest{
		Finder: ui.Node().Name("Show network address settings").Role(ui.Role_ROLE_BUTTON).Finder(),
	}); err != nil {
		return errors.Wrap(err, `failed to expand the "Network" section`)
	}

	// The location of the node could not be stabilized immediately after expanding the network section.
	if _, err := uiSvc.WaitForLocation(ctx, &ui.WaitForLocationRequest{
		Finder: ui.Node().Name("Hidden network").Role(ui.Role_ROLE_TOGGLE_BUTTON).Finder(),
	}); err != nil {
		return errors.Wrap(err, "failed to wait until the node is stabilized")
	}

	// Ensure the "Hidden network" toggle button is visible before interacting with it.
	if _, err := uiSvc.MakeVisible(ctx, &ui.MakeVisibleRequest{
		Finder: ui.Node().Name("Hidden network").Role(ui.Role_ROLE_TOGGLE_BUTTON).Finder(),
	}); err != nil {
		return errors.Wrap(err, "failed to make node visible")
	}

	if _, err := osSettingsSvc.SetToggleOption(ctx, &ossettings.SetToggleOptionRequest{
		ToggleOptionName: "Hidden network",
		Enabled:          true,
	}); err != nil {
		return errors.Wrap(err, "failed to set the 'Hidden network' toggle option")
	}
	return nil
}
