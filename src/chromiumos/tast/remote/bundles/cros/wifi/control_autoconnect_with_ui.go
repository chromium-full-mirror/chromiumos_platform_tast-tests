// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/common/wifi/security/wpa"
	"chromiumos/tast/remote/bundles/cros/wifi/wifiutil"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/remote/wificell/hostapd"
	"chromiumos/tast/services/cros/chrome/uiauto/ossettings"
	"chromiumos/tast/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ControlAutoconnectWithUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify user should be able to specify whether or not a particular network can auto-connect",
		Contacts: []string{
			"chromeos-wifi-champs@google.com",
			"chromeos-sw-engprod@google.com",
			"vivian.tsai@cienet.com",
			"cienet-development@googlegroups.com",
		},
		BugComponent: "b:1131912",
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
	})
}

// ControlAutoconnectWithUI verifies user should be able to specify whether or not a particular network can auto-connect.
func ControlAutoconnectWithUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	ap, err := tf.ConfigureAP(ctx,
		[]hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211g)},
		wpa.NewConfigFactory("testpassphrase", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP)),
	)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	cleanupAPCtx := ctx
	defer tf.DeconfigAP(cleanupAPCtx, ap)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	if _, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap); err != nil {
		s.Fatalf("Failed to connect to AP %q: %v", ap.Config().SSID, err)
	}
	cleanupDUTCtx := ctx
	defer tf.CleanDisconnectDUTFromWifi(cleanupDUTCtx, wificell.DefaultDUT)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	// cleanupCtx is the context with time reserved, used for cleaning up resources other than the AP.
	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	wifiSvc := tf.DUTWifiClient(wificell.DefaultDUT)

	// Ensure the Wifi is enabled to perform the test.
	if err := wifiSvc.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable Wifi feature: ", err)
	}
	// No need to restore the enabled state since Wifi being enabled is the default "good" state for group:wificell.

	if err := cycleWifi(ctx, wifiSvc); err != nil {
		s.Fatal("Failed to cycle Wifi feature: ", err)
	}
	if err := wifiSvc.WaitForConnected(ctx, ap.Config().SSID, true /* expected connected */); err != nil {
		s.Fatal("Failed to verify auto-connect settings: DUT didn't auto connect to the AP: ", err)
	}

	if err := setAutoConnect(ctx, rpcClient, ap.Config().SSID, false /* enabled */); err != nil {
		s.Fatal("Failed to disable auto-connect: ", err)
	}
	if err := cycleWifi(ctx, wifiSvc); err != nil {
		s.Fatal("Failed to cycle Wifi feature: ", err)
	}
	if err := wifiSvc.WaitForConnected(ctx, ap.Config().SSID, false /* expected connected */); err != nil {
		s.Fatal("Failed to verify auto-connect settings: DUT shouldn't connect to the AP with auto-connect disabled: ", err)
	}

	anotherAP, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure another AP: ", err)
	}
	defer tf.DeconfigAP(ctx, anotherAP)
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, anotherAP)
	defer cancel()

	if _, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, anotherAP); err != nil {
		s.Fatalf("Failed to connect to AP %q: %v", anotherAP.Config().SSID, err)
	}
	defer tf.DisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	if err := wifiSvc.WaitForConnected(ctx, anotherAP.Config().SSID, true /* expected connected */); err != nil {
		s.Fatal("Failed to wait for DUT connect to another AP: ", err)
	}

	if err := setAutoConnect(ctx, rpcClient, anotherAP.Config().SSID, false /* enabled */); err != nil {
		s.Fatal("Failed to disable auto-connect on another AP: ", err)
	}
	if err := setAutoConnect(ctx, rpcClient, ap.Config().SSID, true /* enabled */); err != nil {
		s.Fatal("Failed to enable auto-connect on the original AP: ", err)
	}
	if err := cycleWifi(ctx, wifiSvc); err != nil {
		s.Fatal("Failed to cycle Wifi feature: ", err)
	}
	if err := wifiSvc.WaitForConnected(ctx, ap.Config().SSID, true /* expected connected */); err != nil {
		s.Fatal("Failed to verify auto-connect settings: DUT didn't auto connect to the AP with auto-connect enabled: ", err)
	}
}

// cycleWifi refreshes the Wifi feature and then verifies the Wifi network status.
// The Wifi feature will be refreshed by disable Wifi feature and then enable Wifi feature.
func cycleWifi(ctx context.Context, wifiSvc *wificell.WifiClient) error {
	if err := wifiSvc.SetWifiEnabled(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable Wifi feature")
	}

	if err := wifiSvc.SetWifiEnabled(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable Wifi feature")
	}

	return nil
}

func setAutoConnect(ctx context.Context, rpcClient *rpc.Client, ssid string, enabled bool) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	req := &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: ssid,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}

	osSettings := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	if _, err := osSettings.OpenNetworkDetailPage(ctx, req); err != nil {
		return errors.Wrap(err, "failed to open network page")
	}
	defer osSettings.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, func() bool { return retErr != nil }, "set_autoconnect")

	option := &ossettings.SetToggleOptionRequest{
		ToggleOptionName: "Automatically connect to this network",
		Enabled:          enabled,
	}
	if _, err := osSettings.SetToggleOption(ctx, option); err != nil {
		return errors.Wrapf(err, "failed to set toggle option to %v", enabled)
	}
	return nil
}
