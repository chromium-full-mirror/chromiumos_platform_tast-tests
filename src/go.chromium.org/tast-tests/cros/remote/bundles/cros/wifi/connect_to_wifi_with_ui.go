// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/base"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const passphrase = "fourwordsalluppercase"

// connectToWifiWithUITestCase is a simple container for the information on a test case.
type connectToWifiWithUITestCase struct {
	secured bool
	factory security.ConfigFactory
	impl    func(context.Context, connectToWifiWithUITestData) error
}

// connectToWifiWithUITestData is a simple container for the resources common to the different test cases.
type connectToWifiWithUITestData struct {
	secured      bool
	ssid         string
	rpcClient    *rpc.Client
	os           ossettings.OsSettingsServiceClient
	qs           quicksettings.QuickSettingsServiceClient
	uiautomation ui.AutomationServiceClient
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ConnectToWifiWithUI,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests that WiFi can be connected to with different UI surfaces",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.AutomationService",
			wificell.ShillServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{{
			Name: "open_network_using_quick_settings",
			Val: connectToWifiWithUITestCase{
				secured: false,
				factory: base.NewConfigFactory(),
				impl:    fromQuickSettings,
			},
		}, {
			Name: "open_network_using_wifi_settings",
			Val: connectToWifiWithUITestCase{
				secured: false,
				factory: base.NewConfigFactory(),
				impl:    fromWifiSettings,
			},
		}, {
			Name: "open_network_using_wifi_network_page",
			Val: connectToWifiWithUITestCase{
				secured: false,
				factory: base.NewConfigFactory(),
				impl:    fromWifiNetworkPage,
			},
		}, {
			Name: "secured_network_using_quick_settings",
			Val: connectToWifiWithUITestCase{
				secured: true,
				factory: wpa.NewConfigFactory(
					passphrase,
					wpa.Mode(wpa.ModePureWPA),
					wpa.Ciphers(wpa.CipherTKIP),
				),
				impl: fromQuickSettings,
			},
		}, {
			Name: "secured_network_using_wifi_settings",
			Val: connectToWifiWithUITestCase{
				secured: true,
				factory: wpa.NewConfigFactory(
					passphrase,
					wpa.Mode(wpa.ModePureWPA),
					wpa.Ciphers(wpa.CipherTKIP),
				),
				impl: fromWifiSettings,
			},
		}, {
			Name: "secured_network_using_wifi_network_page",
			Val: connectToWifiWithUITestCase{
				secured: true,
				factory: wpa.NewConfigFactory(
					passphrase,
					wpa.Mode(wpa.ModePureWPA),
					wpa.Ciphers(wpa.CipherTKIP),
				),
				impl: fromWifiNetworkPage,
			},
		}},
	})
}

// ConnectToWifiWithUI tests that WiFi can be connected to with different UI surfaces.
func ConnectToWifiWithUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	p := s.Param().(connectToWifiWithUITestCase)

	apInterface, err := tf.ConfigureAP(ctx, []ap.Option{
		ap.Mode(ap.Mode80211a),
		ap.Channel(48),
	}, p.factory)

	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}

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
	defer rpcClient.Close(ctx)

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable Wi-Fi using Shill: ", err)
	}

	chrome := ui.NewChromeServiceClient(rpcClient.Conn)
	defer chrome.Close(ctx, &emptypb.Empty{})

	if _, err := chrome.New(ctx, &ui.NewRequest{
		LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to open Chrome on the DUT: ", err)
	}

	testData := connectToWifiWithUITestData{
		secured:      p.secured,
		ssid:         apInterface.Config().SSID,
		rpcClient:    rpcClient,
		os:           ossettings.NewOsSettingsServiceClient(rpcClient.Conn),
		qs:           quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn),
		uiautomation: ui.NewAutomationServiceClient(rpcClient.Conn),
	}

	defer testData.os.Close(ctx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(ctx, testData.rpcClient.Conn, s.HasError, "ui_tree")

	if err := p.impl(ctx, testData); err != nil {
		s.Fatal("Failed to connect: ", err)
	}
}

func fromQuickSettings(ctx context.Context, testData connectToWifiWithUITestData) error {
	if _, err := testData.qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to the detailed Network within Quick Settings")
	}

	networkFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: testData.ssid}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	if _, err := testData.uiautomation.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: networkFinder}); err != nil {
		return errors.Wrap(err, "failed to click the network button")
	}

	if testData.secured {
		if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Connect"); err != nil {
			return errors.Wrap(err, "failed to configure wifi network")
		}

		if _, err := testData.qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to navigate to the detailed Network within Quick Settings")
		}
	}

	networkConnectedStateFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Connected"}},
			{Value: &ui.NodeWith_HasClass{HasClass: "UnfocusableLabel"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_Ancestor{Ancestor: networkFinder}},
		},
	}
	if _, err := testData.uiautomation.WaitUntilExists(
		ctx, &ui.WaitUntilExistsRequest{Finder: networkConnectedStateFinder}); err != nil {
		return errors.Wrap(err, "failed to find the network connected state")
	}

	return nil
}

func fromWifiSettings(ctx context.Context, testData connectToWifiWithUITestData) error {
	if _, err := testData.os.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to WiFI page within OS Settings")
	}

	networkRegex := fmt.Sprintf(`^Network \d+ of \d+, %s.*`, testData.ssid)
	network := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameRegex{NameRegex: networkRegex}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_GENERIC_CONTAINER}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	if _, err := testData.uiautomation.LeftClick(ctx, &ui.LeftClickRequest{Finder: network}); err != nil {
		return errors.Wrap(err, "failed to select the network from the network list")
	}

	if testData.secured {
		if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Connect"); err != nil {
			return errors.Wrap(err, "failed to fill in the network configuration dialog")
		}
	}

	networkConnectedStateFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_Ancestor{Ancestor: network}},
			{Value: &ui.NodeWith_First{First: true}},
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Connected"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
		},
	}

	if _, err := testData.uiautomation.WaitUntilExists(
		ctx, &ui.WaitUntilExistsRequest{Finder: networkConnectedStateFinder}); err != nil {
		return errors.Wrap(err, "failed to find the network connected state")
	}

	return nil
}

func fromWifiNetworkPage(ctx context.Context, testData connectToWifiWithUITestData) error {
	req := &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: testData.ssid,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}

	if _, err := testData.os.OpenNetworkDetailPage(ctx, req); err != nil {
		return errors.Wrap(err, "failed to to open network page")
	}

	buttonName := "Connect"

	if testData.secured {
		buttonName = "Configure"
	}

	connectButtonNode := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: buttonName}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_BUTTON}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}
	if _, err := testData.uiautomation.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: connectButtonNode}); err != nil {
		return errors.Wrap(err, "failed to click the connect button")
	}

	if testData.secured {
		if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Save"); err != nil {
			return errors.Wrap(err, "failed to configure wifi network")
		}
	}

	networkConnectedStateFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_NameContaining{NameContaining: "Connected"}},
			{Value: &ui.NodeWith_Role{Role: ui.Role_ROLE_STATIC_TEXT}},
			{Value: &ui.NodeWith_First{First: true}},
		},
	}

	if _, err := testData.uiautomation.WaitUntilExists(
		ctx, &ui.WaitUntilExistsRequest{Finder: networkConnectedStateFinder}); err != nil {
		return errors.Wrap(err, "failed to find the network connected state")
	}

	return nil
}
