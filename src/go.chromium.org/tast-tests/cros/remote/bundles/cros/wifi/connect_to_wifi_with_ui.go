// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/base"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const passphrase = "fourwordsalluppercase"

type connectToWifiWithUIImplFunc func(*connectToWifiWithUITestData) action.Action

type connectToWifiWithUITestNetwork struct {
	secured bool
	// channel is a specific frequency range within the 2.4 GHz or 5 GHz bands used by Wi-Fi networks to transmit data,
	// with proper selection reducing interference and improving performance.
	// Note that it is suggested to have only one network per band,
	// or if multiple networks are on the same band we want them on the same channel (b/320811867#comment3).
	channel int
	// routerID is the index of the router to set up the network for.
	// Note that it is suggested to configure a maximum of
	// two networks on a router (b/320811867#comment3).
	routerID wificell.RouterIdx

	ap *wificell.APIface
}

func (n *connectToWifiWithUITestNetwork) securityFactory() security.ConfigFactory {
	if n.secured {
		return wpa.NewConfigFactory(
			passphrase,
			wpa.Mode(wpa.ModePureWPA2),
			wpa.Ciphers2(wpa.CipherCCMP),
		)
	}
	return base.NewConfigFactory()
}

func (n *connectToWifiWithUITestNetwork) apOptions() []hostapd.Option {
	return []hostapd.Option{
		hostapd.Channel(n.channel),
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.HTCaps(hostapd.HTCapHT20),
	}
}

// connectToWifiWithUITestCase is a simple container for the information on a test case.
type connectToWifiWithUITestCase struct {
	impls        []connectToWifiWithUIImplFunc
	testNetworks []*connectToWifiWithUITestNetwork
}

// connectToWifiWithUITestData is a simple container for the resources common to the different test cases.
type connectToWifiWithUITestData struct {
	secured        bool
	ssid           string
	rpcClient      *rpc.Client
	os             ossettings.OsSettingsServiceClient
	qs             quicksettings.QuickSettingsServiceClient
	crosNetworkCfg networkui.CrosNetworkConfigServiceClient
	uiautomation   ui.AutomationServiceClient
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectToWifiWithUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that WiFi can be connected to with different UI surfaces, and can transit from one network to another",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chromeos-connectivity-engprod@google.com",
		},
		BugComponent:   "b:1131912", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > WiFi
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:wificell", "wificell_e2e", "group:release-health", "release-health_wifi"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			"tast.cros.inputs.KeyboardService",
			"tast.cros.ui.AutomationService",
			"tast.cros.networkui.CrosNetworkConfigService",
			wificell.ShillServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesCapture),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{{
			Name: "open_network_using_quick_settings",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&quickSettingsImpl{}).connect(passphrase),
					(&quickSettingsImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: false, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "open_network_using_wifi_settings",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&wifiPageImpl{}).connect(passphrase),
					(&wifiPageImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: false, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "open_network_using_wifi_network_page",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&settingsImpl{}).connect(passphrase),
					(&settingsImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: false, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "secured_network_using_quick_settings",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&quickSettingsImpl{}).connect(passphrase),
					(&quickSettingsImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: true, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "secured_network_using_wifi_settings",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&wifiPageImpl{}).connect(passphrase),
					(&wifiPageImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: true, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "secured_network_using_wifi_network_page",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&settingsImpl{}).connect(passphrase),
					(&settingsImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: true, channel: 1, routerID: 0}, // 2.4G
				},
			},
		}, {
			Name: "open_network_and_secured_network_using_quick_settings",
			Val: connectToWifiWithUITestCase{
				impls: []connectToWifiWithUIImplFunc{
					(&quickSettingsImpl{}).connect(passphrase),
					(&quickSettingsImpl{}).verifyConnected,
					(&mojoImpl{}).verifyConnected,
				},
				testNetworks: []*connectToWifiWithUITestNetwork{
					{secured: false, channel: 1, routerID: 0}, // 2.4 GHz
					{secured: true, channel: 48, routerID: 0}, // 5 GHz
				},
			},
		}},
	})
}

// ConnectToWifiWithUI tests that WiFi can be connected to with different UI surfaces, and can transit from one network to another.
func ConnectToWifiWithUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	p := s.Param().(connectToWifiWithUITestCase)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	chrome := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := chrome.New(ctx, &ui.NewRequest{
		LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to open Chrome on the DUT: ", err)
	}
	defer chrome.Close(cleanupCtx, &emptypb.Empty{})

	for idx, n := range p.testNetworks {
		ap, err := tf.ConfigureAPOnRouterID(ctx, n.routerID, n.apOptions(), n.securityFactory(), false, false)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}

		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()
		defer tf.DeconfigAP(cleanupCtx, ap)

		p.testNetworks[idx].ap = ap
	}

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable Wi-Fi using Shill: ", err)
	}

	testData := &connectToWifiWithUITestData{
		rpcClient:      rpcClient,
		os:             ossettings.NewOsSettingsServiceClient(rpcClient.Conn),
		qs:             quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn),
		crosNetworkCfg: networkui.NewCrosNetworkConfigServiceClient(rpcClient.Conn),
		uiautomation:   ui.NewAutomationServiceClient(rpcClient.Conn),
	}
	defer testData.os.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, testData.rpcClient.Conn, s.HasError, "ui_tree")

	if err := (&quickSettingsImpl{}).networksShown(p.testNetworks)(testData)(ctx); err != nil {
		s.Fatal("Failed to verify network is shown in the Quick Settings: ", err)
	}

	for _, network := range p.testNetworks {
		testData.ssid = network.ap.Config().SSID
		testData.secured = network.secured

		for _, impl := range p.impls {
			if err := impl(testData)(ctx); err != nil {
				s.Fatal("Failed to implement: ", err)
			}
		}
	}
}

type mojoImpl struct{}

func (impl *mojoImpl) verifyConnected(testData *connectToWifiWithUITestData) action.Action {
	return func(ctx context.Context) error {
		resp, err := testData.crosNetworkCfg.GetNetworkStateList(ctx, &networkui.NetworkFilter{
			Network: networkui.NetworkType_WIFI,
			Filter:  networkui.FilterType_CONFIGURED,
		})
		if err != nil {
			return errors.Wrap(err, "failed to get network state list through Mojo API")
		}

		if found := slices.IndexFunc(resp.GetProperties(), func(property *networkui.NetworkStateProperties) bool {
			if property.GetName() != testData.ssid {
				return false
			}
			switch property.GetConnectionState() {
			case networkui.ConnectionState_ONLINE, networkui.ConnectionState_CONNECTED, networkui.ConnectionState_PORTAL:
				return true
			default:
				return false
			}
		}) >= 0; !found {
			return errors.Wrapf(err, "failed to verify the WiFi network %q is connected through Mojo API", testData.ssid)
		}
		return nil
	}
}

type quickSettingsImpl struct{}

func (impl *quickSettingsImpl) networkItemFinder(data *connectToWifiWithUITestData) *ui.Finder {
	return ui.Node().NameContaining(data.ssid).Nth(0).Finder()
}

func (impl *quickSettingsImpl) connectedLabel(data *connectToWifiWithUITestData) *ui.Finder {
	return ui.Node().NameRegex("^Connected").HasClass("UnfocusableLabel").Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(impl.networkItemFinder(data)).Finder()
}

func (impl *quickSettingsImpl) connect(passphrase string) func(testData *connectToWifiWithUITestData) action.Action {
	return func(testData *connectToWifiWithUITestData) action.Action {
		return func(ctx context.Context) error {
			if _, err := testData.qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
				return errors.Wrap(err, "failed to navigate to the detailed Network within Quick Settings")
			}

			if _, err := testData.uiautomation.DoDefault(ctx, &ui.DoDefaultRequest{
				Finder: impl.networkItemFinder(testData),
			}); err != nil {
				return errors.Wrap(err, "failed to click the network button")
			}

			if testData.secured {
				if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Connect"); err != nil {
					return errors.Wrap(err, "failed to configure wifi network")
				}
			}
			return nil
		}
	}
}

func (impl *quickSettingsImpl) verifyConnected(testData *connectToWifiWithUITestData) action.Action {
	return func(ctx context.Context) error {
		if _, err := testData.qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to navigate to the detailed Network within Quick Settings")
		}

		if _, err := testData.uiautomation.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
			Finder: impl.connectedLabel(testData),
		}); err != nil {
			return errors.Wrap(err, "failed to find the network connected state")
		}
		return nil
	}
}

func (impl *quickSettingsImpl) networksShown(networks []*connectToWifiWithUITestNetwork) func(testData *connectToWifiWithUITestData) action.Action {
	return func(testData *connectToWifiWithUITestData) action.Action {
		return func(ctx context.Context) error {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			if _, err := testData.qs.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
				return errors.Wrap(err, "failed to navigate to network detailed view within Quick Setting")
			}
			defer testData.qs.Hide(cleanupCtx, &emptypb.Empty{})

			// Retry on checking the available WiFi networks as it takes a moment for scan and update the UI.
			return testing.Poll(ctx, func(ctx context.Context) error {
				resp, err := testData.qs.AvailableWifiNetworks(ctx, &emptypb.Empty{})
				if err != nil {
					testing.PollBreak(errors.Wrap(err, "failed to retrieve the available WiFi network from Quick Settings"))
				}

				// Verify networks are shown in the Quick Settings.
				for _, network := range networks {
					ssid := network.ap.Config().SSID
					if !slices.Contains(resp.GetSsids(), ssid) {
						return errors.Errorf("network %q is not shown in Quick Settings", ssid)
					}
				}
				return nil
			}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second})
		}
	}
}

type wifiPageImpl struct{}

func (impl *wifiPageImpl) networkItemFinder(data *connectToWifiWithUITestData) *ui.Finder {
	networkRegex := fmt.Sprintf(`^Network \d+ of \d+, %s.*`, data.ssid)
	return ui.Node().NameRegex(networkRegex).Role(ui.Role_ROLE_GENERIC_CONTAINER).Nth(0).Finder()
}

func (impl *wifiPageImpl) connectedLabel(data *connectToWifiWithUITestData) *ui.Finder {
	return ui.Node().NameRegex(`^Connected`).Role(ui.Role_ROLE_STATIC_TEXT).Nth(0).Ancestor(impl.networkItemFinder(data)).Finder()
}

func (impl *wifiPageImpl) connect(passphrase string) func(testData *connectToWifiWithUITestData) action.Action {
	return func(testData *connectToWifiWithUITestData) action.Action {
		return func(ctx context.Context) error {
			if _, err := testData.os.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
				return errors.Wrap(err, "failed to navigate to WiFI page within OS Settings")
			}

			req := &ossettings.EvalJSWithShadowPiercerRequest{
				Expression: fmt.Sprintf(`var items = shadowPiercingQueryAll("div#itemTitle");
				var targetWifi = undefined;
				for (let i = 0; i < items.length; i++) {
					if (items[i].textContent.trim() === %q) {
						targetWifi = items[i];
						items[i].click();
						break;
					}
				}
				targetWifi != undefined;`, testData.ssid),
			}

			// Attempting to click an entry using JavaScript instead of other approaches due to:
			// 1. The page contains multiple entries whose order can change based on the scanning
			// 	status, making a regular UI left-click to be flaky.
			// 2. Each entry is wrapped in a Role_ROLE_GENERIC_CONTAINER, which is a <div> that
			// 	may not have an action listener registered, causing the regular DoDefault RPC call
			// 	does not achieve the goal.
			if _, err := testData.os.EvalJSWithShadowPiercer(ctx, req); err != nil {
				return errors.Wrap(err, "failed to select the network from the network list")
			}

			if testData.secured {
				if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Connect"); err != nil {
					return errors.Wrap(err, "failed to fill in the network configuration dialog")
				}
			}

			return nil
		}
	}
}

func (impl *wifiPageImpl) verifyConnected(testData *connectToWifiWithUITestData) action.Action {
	return func(ctx context.Context) error {
		if _, err := testData.uiautomation.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
			Finder: impl.connectedLabel(testData),
		}); err != nil {
			return errors.Wrap(err, "failed to find the network connected state")
		}
		return nil
	}
}

type settingsImpl struct{}

func (impl *settingsImpl) uiRoot() *ui.Finder {
	return ui.Node().NameRegex(`^Settings`).Role(ui.Role_ROLE_ROOT_WEB_AREA).Finder()
}

func (impl *settingsImpl) networkItemFinder(testData *connectToWifiWithUITestData) *ui.Finder {
	networkRegex := fmt.Sprintf(`^Network \d+ of \d+, %s.*`, testData.ssid)
	return ui.Node().NameRegex(networkRegex).Role(ui.Role_ROLE_GENERIC_CONTAINER).Nth(0).Finder()
}

func (impl *settingsImpl) connectedLabel() *ui.Finder {
	return ui.Node().NameRegex(`^Connected`).Role(ui.Role_ROLE_STATIC_TEXT).Nth(0).Ancestor(impl.uiRoot()).Finder()
}

func (impl *settingsImpl) connect(passphrase string) func(testData *connectToWifiWithUITestData) action.Action {
	return func(testData *connectToWifiWithUITestData) action.Action {
		return func(ctx context.Context) error {
			req := &ossettings.OpenNetworkDetailPageRequest{
				NetworkName: testData.ssid,
				NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
			}

			if _, err := testData.os.OpenNetworkDetailPage(ctx, req); err != nil {
				return errors.Wrap(err, "failed to to open network page")
			}

			if testData.secured {
				configButtonNode := ui.Node().Name("Configure").Role(ui.Role_ROLE_BUTTON).Nth(0).Finder()
				if _, err := testData.uiautomation.DoDefault(
					ctx, &ui.DoDefaultRequest{Finder: configButtonNode}); err != nil {
					return errors.Wrap(err, "failed to click the connect button")
				}
				if err := wifiutil.ConfigureWifiNetwork(ctx, testData.uiautomation, testData.rpcClient.Conn, passphrase, "Save"); err != nil {
					return errors.Wrap(err, "failed to configure wifi network")
				}
			}
			connectButtonNode := ui.Node().Name("Connect").Role(ui.Role_ROLE_BUTTON).Nth(0).Finder()
			if _, err := testData.uiautomation.DoDefault(
				ctx, &ui.DoDefaultRequest{Finder: connectButtonNode}); err != nil {
				return errors.Wrap(err, "failed to click the connect button")
			}
			return nil
		}
	}
}

func (impl *settingsImpl) verifyConnected(testData *connectToWifiWithUITestData) action.Action {
	return func(ctx context.Context) error {
		if _, err := testData.uiautomation.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
			Finder: impl.connectedLabel(),
		}); err != nil {
			return errors.Wrap(err, "failed to find the network connected state")
		}
		return nil
	}
}
