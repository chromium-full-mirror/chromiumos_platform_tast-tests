// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	tdreq "chromiumos/tast/common/testdevicerequirements"
	"chromiumos/tast/common/wifi/security"
	"chromiumos/tast/common/wifi/security/base"
	"chromiumos/tast/common/wifi/security/wpa"
	"chromiumos/tast/remote/bundles/cros/wifi/wifiutil"
	"chromiumos/tast/remote/wificell"
	ap "chromiumos/tast/remote/wificell/hostapd"
	"chromiumos/tast/rpc"
	"chromiumos/tast/services/cros/chrome/uiauto/ossettings"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
)

const passphrase = "fourwordsalluppercase"

type securityStruct struct {
	secured bool
	factory security.ConfigFactory
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectFromOsSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a user can connect to WiFi from ChromeOS Settings",
		Contacts: []string{
			"cros-connectivity@google.com",
			"tjohnsonkanu@google.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.AutomationService",
			wificell.TFServiceName,
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixtWithCapture",
		Requirements: []string{
			tdreq.WiFiProcPassFW,
			tdreq.WiFiProcPassAVL,
			tdreq.WiFiProcPassAVLBeforeUpdates,
			tdreq.WiFiProcPassMatfunc,
			tdreq.WiFiProcPassMatfuncBeforeUpdates,
		},
		Params: []testing.Param{{
			Name: "open",
			Val: securityStruct{
				secured: false,
				factory: base.NewConfigFactory(),
			},
		}, {
			Name: "secured",
			Val: securityStruct{
				secured: true,
				factory: wpa.NewConfigFactory(
					passphrase,
					wpa.Mode(wpa.ModePureWPA),
					wpa.Ciphers(wpa.CipherTKIP),
				),
			},
		}},
	})
}

func ConnectFromOsSettings(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	p := s.Param().(securityStruct)

	apInterface, err := tf.ConfigureAP(ctx, []ap.Option{
		ap.Mode(ap.Mode80211a),
		ap.Channel(48),
	}, p.factory)

	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	ssid := apInterface.Config().SSID

	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, apInterface); err != nil {
			s.Error("Failed to deconfigure AP: ", err)
		}
	}(ctx)

	ctx, cancel := tf.ReserveForDeconfigAP(ctx, apInterface)
	defer cancel()

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to create RPC client: ", err)
	}

	chrome := ui.NewChromeServiceClient(rpcClient.Conn)
	os := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	uiautomation := ui.NewAutomationServiceClient(rpcClient.Conn)
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)

	defer rpcClient.Close(ctx)
	defer os.Close(ctx, &emptypb.Empty{})
	defer chrome.Close(ctx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(ctx, rpcClient.Conn, s.HasError, "ui_tree")

	if _, err = chrome.New(ctx, &ui.NewRequest{
		LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to open Chrome on the DUT: ", err)
	}

	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable Wi-Fi using Shill: ", err)
	}

	req := &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: ssid,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}

	if _, err := os.OpenNetworkDetailPage(ctx, req); err != nil {
		s.Fatal("Failed to to open network page: ", err)
	}

	isSecured := s.Param().(securityStruct).secured
	buttonName := "Connect"

	if isSecured {
		buttonName = "Configure"
	}

	connectButtonNode := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: buttonName}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
		},
	}
	if _, err := uiautomation.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: connectButtonNode}); err != nil {
		s.Fatal("Failed to click the connect button: ", err)
	}

	if isSecured {
		if err := wifiutil.ConfigureWifiNetwork(ctx, uiautomation, rpcClient.Conn, passphrase, "Save"); err != nil {
			s.Fatal("Failed to configure wifi network: ", err)
		}
	}

	networkConnectedStateFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Connected"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	if _, err := uiautomation.WaitUntilExists(
		ctx, &ui.WaitUntilExistsRequest{Finder: networkConnectedStateFinder}); err != nil {
		s.Fatal("Failed to find the network connected state: ", err)
	}
}
