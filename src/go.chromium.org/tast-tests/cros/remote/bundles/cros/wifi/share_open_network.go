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
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShareOpenNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the open network can be shared with all users",
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
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			"tast.cros.ui.AutomationService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      5 * time.Minute,
	})
}

// ShareOpenNetwork verifies that open networks once connected can be shared with all users.
func ShareOpenNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	// Login as device owner, join a network and then verify the network is connected.
	if err := loginAndVerify(ctx, rpcClient, &ui.NewRequest{}, ap.Config().SSID, true /* shouldJoinWifi */); err != nil {
		s.Fatal("Failed to confirm network is available for owner users: ", err)
	}

	// Verify that the network is shared with secondary user
	// by login as a secondary user and check if the network is listed in the known network.
	if err := loginAndVerify(ctx, rpcClient, &ui.NewRequest{
		LoginMode:   ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		Credentials: &ui.NewRequest_Credentials{Username: "testuser2@gmail.com", Password: "test0000"},
		KeepState:   true,
	}, ap.Config().SSID, false /* shouldJoinWifi */); err != nil {
		s.Fatal("Failed to verify the network is shared with secondary user: ", err)
	}

	// Verify that the network is shared with guest user
	// by login as guest user and check if the network is listed in the known network.
	if err := loginAndVerify(ctx, rpcClient, &ui.NewRequest{
		LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
		KeepState: true,
	}, ap.Config().SSID, false /* shouldJoinWifi */); err != nil {
		s.Fatal("Failed to verify the network is shared with guest user: ", err)
	}
}

// loginAndVerify logs in and verifies the specified network satisfies the following criteria:
//  1. Labeled as "Connected" in QuickSettings.
//  2. Shown in known network list, in OSSettings.
//
// This method will also join an unsecured network if |shouldJoinWifi| is specified.
func loginAndVerify(ctx context.Context, rpcClient *rpc.Client, startCrRequest *ui.NewRequest, ssid string, shouldJoinWifi bool) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, startCrRequest); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, func() bool { return retErr != nil }, "ui_tree")

	if shouldJoinWifi {
		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
		if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
			Ssid:     ssid,
			Security: &wifi.JoinWifiRequest_None{},
		}); err != nil {
			return errors.Wrap(err, "failed to join WiFi from Quick Settings")
		}
	}

	qs := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	if _, err := qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to the detailed Network within Quick Settings")
	}

	uiauto := ui.NewAutomationServiceClient(rpcClient.Conn)
	networkFinder := ui.Node().NameContaining(ssid).Role(ui.Role_ROLE_BUTTON).Finder()
	networkConnectedStateFinder := ui.Node().NameContaining("Connected").Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(networkFinder).Finder()
	if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: networkConnectedStateFinder}); err != nil {
		return errors.Wrap(err, `failed to verify the network is labeled as "Connected"`)
	}

	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
	}); err != nil {
		return errors.Wrap(err, "failed to verify the network is listed in known networks")
	}

	return nil
}
