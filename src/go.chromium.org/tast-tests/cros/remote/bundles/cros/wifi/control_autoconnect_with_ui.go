// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// A sub test sets up the auto-connect property for up to 2 networks (series of UI actions)
// and wait for the connect status of 2 networks to be expected (wireless connection could take a while to respond)
const subTestTimeout = 2 * time.Minute

type controlAutoconnectWithUIParam struct {
	testFunc func(context.Context, *wificell.TestFixture) error
	wpaMode  wpa.ModeEnum
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ControlAutoconnectWithUI,
		Desc: "Verify user should be able to specify whether or not a particular network can auto-connect after turning WiFi off/on, rebooting and waking up from sleep",
		Contacts: []string{
			"alfredyu@cienet.com",
			"vivian.chen@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleInDevelopment,
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:           []string{"group:wificell", "wificell_e2e_unstable"},
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.wifi.WifiService",
			wifiutil.FaillogServiceName,
			"tast.cros.platform.UpstartService",
		},
		SoftwareDeps:    []string{"chrome"},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesRouters),
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{
			{
				Name: "cycle_wifi_wpa2",
				Val: controlAutoconnectWithUIParam{
					testFunc: cycleWifi,
					wpaMode:  wpa.ModePureWPA2,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout, // Each test performs 4 sub tests.
			}, {
				Name: "cycle_wifi_wpa3",
				// Marvell Wi-Fi chip does not support WPA3 (b/313946503).
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				Val: controlAutoconnectWithUIParam{
					testFunc: cycleWifi,
					wpaMode:  wpa.ModePureWPA3,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout, // Each test performs 4 sub tests.

			}, {
				Name: "reboot_dut_wpa2",
				Val: controlAutoconnectWithUIParam{
					testFunc: rebootDUT,
					wpaMode:  wpa.ModePureWPA2,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout + wificell.DUTRebootTimeout, // Each test performs 4 sub tests.
			}, {
				Name:              "reboot_dut_wpa3",
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				Val: controlAutoconnectWithUIParam{
					testFunc: rebootDUT,
					wpaMode:  wpa.ModePureWPA3,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout + wificell.DUTRebootTimeout, // Each test performs 4 sub tests.
			}, {
				Name: "re_login_wpa2",
				Val: controlAutoconnectWithUIParam{
					testFunc: logoutDUT,
					wpaMode:  wpa.ModePureWPA2,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout, // Each test performs 4 sub tests.
			}, {
				Name:              "re_login_wpa3",
				ExtraHardwareDeps: hwdep.D(hwdep.WifiNotMarvell()),
				Val: controlAutoconnectWithUIParam{
					testFunc: logoutDUT,
					wpaMode:  wpa.ModePureWPA3,
				},
				Timeout: 3*time.Minute + 4*subTestTimeout, // Each test performs 4 sub tests.
			},
		},
	})
}

type apIdentifier int

const (
	primaryAP apIdentifier = iota
	anotherAP
	notUsedAP1 // For simulate multiple Wifi networks available within the range.
	notUsedAP2 // For simulate multiple Wifi networks available within the range.
)

// ControlAutoconnectWithUI verifies user should be able to specify whether or not a particular
// network can auto-connect after turning WiFi off/on, login/logout, rebooting and waking up from sleep.
func ControlAutoconnectWithUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap := map[apIdentifier]*apUtil{}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Start a chrome session before configuring access points to guarantee that all
	// of the expected certificates are loaded to prevent issues with encryption.
	cr := ui.NewChromeServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	param := s.Param().(controlAutoconnectWithUIParam)

	const networkPsk = "password"
	apConfig := wpa.NewConfigFactory(networkPsk, wpa.Mode(param.wpaMode), wpa.Ciphers2(wpa.CipherCCMP))
	for _, cfg := range []struct {
		// id is the identifier of the network.
		id apIdentifier
		// options is the configuration to start hostapd on a router.
		// Note that `hostapd.Channel` is essential and it is suggested to have only one network on a channel (b/320811867#comment3).
		options []hostapd.Option
		// routerID is the index of the router to set up the network for.
		// Note that it is suggested to configure a maximum of two networks on a router (b/320811867#comment3).
		routerID wificell.RouterIdx
	}{
		{
			id: primaryAP,
			options: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.PMF(hostapd.PMFOptional),
				hostapd.Channel(157),
			},
			routerID: 0,
		}, {
			id: anotherAP,
			options: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.PMF(hostapd.PMFOptional),
				hostapd.Channel(36),
			},
			routerID: 1,
		}, {
			id: notUsedAP1,
			options: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.PMF(hostapd.PMFOptional),
				hostapd.Channel(11),
			},
			routerID: 0,
		}, {
			id: notUsedAP2,
			options: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.PMF(hostapd.PMFOptional),
				hostapd.Channel(1),
			},
			routerID: 1,
		},
	} {
		accessPoint, err := tf.ConfigureAPOnRouterID(ctx, cfg.routerID, cfg.options, apConfig, false, false)
		if err != nil {
			s.Fatal("Failed to setup an AP: ", err)
		}

		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, accessPoint)
		defer cancel()
		defer tf.DeconfigAP(cleanupCtx, accessPoint)

		ap[cfg.id] = &apUtil{APIface: accessPoint, tf: tf, networkPsk: networkPsk}
	}
	cleanupCtx = ctx
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	// Checking if the known network list is empty as the precondition of this test.
	settingsSvc := ossettings.NewOsSettingsServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
	resp, err := settingsSvc.KnownWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to acquire the known network list: ", err)
	}
	if len(resp.GetSsids()) != 0 {
		s.Fatal("Failed to verify DUT is ready for test: known network list is not empty")
	}

	// Preparing the primary known network for upcoming tests.
	if err := ap[primaryAP].connectAndWaitForItToBeKnownNetwork(ctx); err != nil {
		s.Fatalf("Failed to connect to AP %q: %v", ap[primaryAP].Config().SSID, err)
	}

	for _, test := range []struct {
		description string
		setup       action.Action
		verify      action.Action
	}{
		{
			description: "the network should automatically connect since auto-connect is enabled by default",
			setup:       func(ctx context.Context) error { return nil },
			verify:      ap[primaryAP].ensureConnectedState(true),
		}, {
			description: "the network should not automatically connect when auto-connect is disabled",
			setup:       ap[primaryAP].setAutoConnect(false),
			verify:      ap[primaryAP].ensureConnectedState(false),
		}, {
			description: "a network with auto-connect enabled should be automatically connected to even when we are connected to a network with auto-connect disabled",
			setup: action.Combine("setup another known network and the auto-connect property of each known network",
				ap[anotherAP].connectAndWaitForItToBeKnownNetwork,
				ap[anotherAP].setAutoConnect(false),
				ap[primaryAP].setAutoConnect(true),
			),
			verify: ap[primaryAP].ensureConnectedState(true),
		}, {
			description: "networks with auto-connect disabled should not be automatically connected to",
			setup: action.Combine("setup the auto-connect property of each known network",
				ap[anotherAP].setAutoConnect(false),
				ap[primaryAP].setAutoConnect(false),
			),
			verify: action.Combine("verify connected status of each known network",
				ap[primaryAP].ensureConnectedState(false),
				ap[anotherAP].ensureConnectedState(false),
			),
		},
	} {
		s.Run(ctx, test.description, func(ctx context.Context, s *testing.State) {
			if err := startChromeAndPerformAction(ctx, tf, test.setup); err != nil {
				s.Fatal("Failed to setup for test: ", err)
			}

			if err := param.testFunc(ctx, tf); err != nil {
				s.Fatal("Failed to perform the action that triggers WiFi to be restarted: ", err)
			}

			if err := startChromeAndPerformAction(ctx, tf, test.verify); err != nil {
				s.Fatal("Failed to verify the test: ", err)
			}
		})
	}
}

// startChromeAndPerformAction will either re-use the existing Chrome session if one exists or
// start a new Chrome session. Afterwards, this function will execute |action|.
func startChromeAndPerformAction(ctx context.Context, tf *wificell.TestFixture, action action.Action) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := ui.NewChromeServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{KeepState: true, TryReuseSession: true}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	return action(ctx)
}

type apUtil struct {
	*wificell.APIface
	tf         *wificell.TestFixture
	networkPsk string
}

func (ap *apUtil) connectAndWaitForItToBeKnownNetwork(ctx context.Context) error {
	wifiSvc := wifi.NewWifiServiceClient(ap.tf.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
		Ssid:     ap.Config().SSID,
		Security: &wifi.JoinWifiRequest_Psk{Psk: ap.networkPsk},
	}); err != nil {
		return errors.Wrapf(err, "failed to join Wi-Fi %q from Quick Settings", ap.Config().SSID)
	}
	return nil
}

func (ap *apUtil) ensureConnectedState(state bool) action.Action {
	return func(ctx context.Context) error {
		if state {
			if err := ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
				return err
			}

			wifiSvc := wifi.NewWifiServiceClient(ap.tf.DUTRPC(wificell.DefaultDUT).Conn)
			_, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
				Ssids:   []string{ap.Config().SSID},
				Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
			})
			return err
		}

		// There could be a short latency before auto-connect to be effective,
		// the network is likely to be not connected at first when the WiFi is just came back, then automatically connect back shortly after.
		// Hence, "AP not connected" is verified by "wait for it to be connected and expecting an error".
		if err := ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
			instantCheckCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			// Call the same RPC again to check it's not connected as an error can indicates other failures.
			return ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(instantCheckCtx, ap.Config().SSID, false /* expectedValue */)
		}
		return errors.Errorf("the AP %q is connected", ap.Config().SSID)
	}
}

func (ap *apUtil) setAutoConnect(enable bool) action.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		req := &ossettings.OpenNetworkDetailPageRequest{
			NetworkName: ap.Config().SSID,
			NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
		}

		rpcClient := ap.tf.DUTRPC(wificell.DefaultDUT)
		osSettings := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
		if _, err := osSettings.OpenNetworkDetailPage(ctx, req); err != nil {
			return errors.Wrap(err, "failed to open network page")
		}
		defer osSettings.Close(cleanupCtx, &emptypb.Empty{})
		defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, func() bool { return retErr != nil }, "set_auto_connect")

		option := &ossettings.SetToggleOptionRequest{
			ToggleOptionName: "Automatically connect to this network",
			Enabled:          enable,
		}
		if _, err := osSettings.SetToggleOption(ctx, option); err != nil {
			return errors.Wrapf(err, "failed to set toggle option to %v", enable)
		}
		return nil
	}
}

// cycleWifi refreshes the Wifi feature and then verifies the Wifi network status.
// The Wifi feature will be refreshed by disabling Wifi feature and then enabling Wifi feature.
func cycleWifi(ctx context.Context, tf *wificell.TestFixture) error {
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable Wifi feature")
	}
	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable Wifi feature")
	}
	return nil
}

func rebootDUT(ctx context.Context, tf *wificell.TestFixture) error {
	return tf.RebootDUT(ctx, wificell.DefaultDUT)
}

// logoutDUT logouts the DUT by restarting the UI.
func logoutDUT(ctx context.Context, tf *wificell.TestFixture) error {
	conn := tf.DUTRPC(wificell.DefaultDUT).Conn
	upstartService := platform.NewUpstartServiceClient(conn)
	_, err := upstartService.RestartJob(ctx, &platform.RestartJobRequest{JobName: "ui"})
	return err
}
