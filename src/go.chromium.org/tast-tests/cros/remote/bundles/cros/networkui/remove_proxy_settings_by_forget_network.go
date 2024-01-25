// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type removeProxyTestParam struct {
	shareOptionForPrimaryUser   wifi.JoinWifiRequest_SharedOption
	shareOptionForSecondaryUser wifi.JoinWifiRequest_SharedOption
}

func (r *removeProxyTestParam) networkIsShared() bool {
	return r.shareOptionForSecondaryUser == wifi.JoinWifiRequest_TurnOn
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoveProxySettingsByForgetNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that if a user forgets a network, the proxy settings are removed for that network as well",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"chromeos-connectivity-cienet-external@google.com",
			"cj.tsai@cienet.com",
		},
		BugComponent:   "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:wificell", "wificell_e2e_unstable"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wificell.BrowserChromeServiceName,
			wificell.WifiUIServiceName,
			wificell.OsSettingsServiceName,
			wificell.ProxyFixtServiceDepsProxySetting,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		// Each test perform login 5 times, and each login could take up to 2 minutes.
		Timeout: 2*time.Minute + 5*(2*time.Minute),
		Params: []testing.Param{
			{
				Name: "shared_network",
				Val: &removeProxyTestParam{
					shareOptionForPrimaryUser:   wifi.JoinWifiRequest_TurnOn,
					shareOptionForSecondaryUser: wifi.JoinWifiRequest_TurnOn,
				},
			}, {
				Name: "non_shared_network",
				Val: &removeProxyTestParam{
					shareOptionForPrimaryUser:   wifi.JoinWifiRequest_TurnOff,
					shareOptionForSecondaryUser: wifi.JoinWifiRequest_TurnOff,
				},
			}, {
				Name: "shared_from_other_user",
				Val: &removeProxyTestParam{
					shareOptionForPrimaryUser:   wifi.JoinWifiRequest_TurnOff,
					shareOptionForSecondaryUser: wifi.JoinWifiRequest_TurnOn,
				},
			}, {
				Name: "non_shared_from_other_user",
				Val: &removeProxyTestParam{
					shareOptionForPrimaryUser:   wifi.JoinWifiRequest_TurnOn,
					shareOptionForSecondaryUser: wifi.JoinWifiRequest_TurnOff,
				},
			},
		},
	})
}

// RemoveProxySettingsByForgetNetwork verifies that if a user forgets a network,
// the proxy settings are removed for that network as well.
func RemoveProxySettingsByForgetNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	params := s.Param().(*removeProxyTestParam)

	var (
		primaryUser   = &ui.NewRequest_Credentials{Username: "pirmaryUser@gmail.com", Password: "testpass"}
		secondaryUser = &ui.NewRequest_Credentials{Username: "secondaryUser@gmail.com", Password: "testpass"}

		ssid     = hostapd.RandomSSID("remove_proxy_test")
		password = "password"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Clean and disconnect the network after the test is done.
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)
	defer tf.DeconfigAllAPs(cleanupCtx)

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	// Isolate the step to leverage `defer` pattern.
	func(ctx context.Context) {
		if _, err := crSvc.New(ctx, &ui.NewRequest{
			LoginMode:   ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			Credentials: primaryUser,
		}); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

		options := append(wificell.DefaultWPA3NetworkAPOptions(), hostapd.SSID(ssid))
		securityConfig := wpa.NewConfigFactory(password, wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP))
		if _, err := tf.ConfigureAP(ctx, options, securityConfig); err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
	}(ctx)

	for _, test := range []struct {
		name      string
		loginUser *ui.NewRequest_Credentials
		actions   []action.Action
	}{
		{
			name:      "connect_and_set_proxy_by_primary_user",
			loginUser: primaryUser,
			actions: []action.Action{
				connectToNetwork(rpcClient, ssid, password, params.shareOptionForPrimaryUser),
				setProxySettings(rpcClient, ssid),
				verifyProxySettings(rpcClient, ssid, true /* shouldBeRemembered */),
			},
		}, {
			name:      "override_shared_option_by_secondary_user",
			loginUser: secondaryUser,
			actions: []action.Action{
				connectToNetwork(rpcClient, ssid, password, params.shareOptionForSecondaryUser),
			},
		}, {
			name:      "forget_network_by_primary_user",
			loginUser: primaryUser,
			actions: []action.Action{
				forgetNetwork(rpcClient, ssid),
			},
		}, {
			name:      "verify_network_is_forgotten_by_secondary_user",
			loginUser: secondaryUser,
			actions: []action.Action{
				// The network has been forgotten by other user at this point,
				// and if the network is shared between users, the network should be forgotten for this user as well.
				verifyNetworkIsForgotten(rpcClient, ssid, params.networkIsShared() /* isForgotten */),
			},
		}, {
			name:      "verify_proxy_settings_are_removed_after_rejoin_network_by_primary_user",
			loginUser: primaryUser,
			actions: []action.Action{
				connectToNetwork(rpcClient, ssid, password, wifi.JoinWifiRequest_Default),
				verifyProxySettings(rpcClient, ssid, false /* shouldBeRemembered */),
			},
		},
	} {
		// Isolate the steps to leverage `defer` pattern.
		func(ctx context.Context) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			if _, err := crSvc.New(ctx, &ui.NewRequest{
				LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
				Credentials:     test.loginUser,
				KeepState:       true,
				TryReuseSession: true,
			}); err != nil {
				s.Fatal("Failed to start Chrome: ", err)
			}
			defer func(ctx context.Context) {
				if s.HasError() {
					ui.NewChromeUIServiceClient(rpcClient.Conn).DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: test.name})
				}
				crSvc.Close(ctx, &emptypb.Empty{})
			}(cleanupCtx)

			for _, action := range test.actions {
				if err := action(ctx); err != nil {
					s.Fatalf("Failed to do action for test %q: %v", test.name, err)
				}
			}
		}(ctx)
	}

}

// connectToNetwork connects to the network and set as shared if needed.
func connectToNetwork(rpcClient *rpc.Client, ssid, password string, shared wifi.JoinWifiRequest_SharedOption) action.Action {
	return func(ctx context.Context) error {
		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
		joinWifiRequest := &wifi.JoinWifiRequest{
			Ssid:                ssid,
			Security:            &wifi.JoinWifiRequest_None{},
			ShareWithOtherUsers: shared,
		}
		if password != "" {
			joinWifiRequest.Security = &wifi.JoinWifiRequest_Psk{Psk: password}
		}
		if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, joinWifiRequest); err != nil {
			return errors.Wrap(err, "failed to join WiFi")
		}
		return nil
	}
}

// setProxySettings sets the proxy settings.
func setProxySettings(rpcClient *rpc.Client, ssid string) action.Action {
	return func(ctx context.Context) error {
		proxySettingSvc := networkui.NewProxySettingServiceClient(rpcClient.Conn)
		if _, err := proxySettingSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
			Configs:   wificell.DefaultProxyConfig(ssid),
			LoginMode: networkui.LoginMode_LoggedIn,
		}); err != nil {
			return errors.Wrap(err, "failed to set proxy settings")
		}
		return nil
	}
}

// verifyProxySettings verifies the proxy settings is stored by cros-network-config or
// being reset as default by OS-settings after forgetting the networks.
func verifyProxySettings(rpcClient *rpc.Client, ssid string, shouldBeRemembered bool) action.Action {
	return func(ctx context.Context) error {
		proxySettingSvc := networkui.NewProxySettingServiceClient(rpcClient.Conn)
		resp, err := proxySettingSvc.FetchProxySettings(ctx, &networkui.FetchProxySettingsRequest{
			NetworkInfo: &networkui.NetworkInfo{Value: &networkui.NetworkInfo_WifiSsid{WifiSsid: ssid}},
			FetchSource: networkui.FetchProxySettingsRequest_CrosNetworkConfig,
			LoginMode:   networkui.LoginMode_LoggedIn,
		})
		if err != nil {
			return errors.Wrap(err, "failed to fetch proxy settings")
		}

		if shouldBeRemembered {
			if diff := cmp.Diff(resp, wificell.DefaultProxyConfig(ssid), protocmp.Transform()); diff != "" {
				return errors.Errorf("unexpected proxy values (-got +want): %s", diff)
			}
		} else if resp.ProxyConnectionType != networkui.ProxyConnectionType_DirectInternetConnection {
			return errors.Errorf("unexpected proxy values, got: %v; want: %v", resp, networkui.ProxyConnectionType_DirectInternetConnection)
		}

		return nil
	}
}

// forgetNetwork forgets the known network.
func forgetNetwork(rpcClient *rpc.Client, ssid string) action.Action {
	return func(ctx context.Context) error {
		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{ssid},
			Control: wifi.KnownNetworksControlsRequest_Forget,
		}); err != nil {
			return errors.Wrap(err, "failed to forget network")
		}

		// Ensure networks are forgotten.
		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{ssid},
			Control: wifi.KnownNetworksControlsRequest_WaitUntilGone,
		}); err != nil {
			return errors.Wrap(err, "failed to wait until network is gone")
		}
		return nil
	}
}

// verifyNetworkIsForgotten verifies that the network is not exist in the known network page.
func verifyNetworkIsForgotten(rpcClient *rpc.Client, ssids string, isForgotten bool) action.Action {
	control := wifi.KnownNetworksControlsRequest_WaitUntilGone
	if !isForgotten {
		control = wifi.KnownNetworksControlsRequest_WaitUntilExist
	}
	return func(ctx context.Context) error {
		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{ssids},
			Control: control,
		}); err != nil {
			return errors.Wrap(err, "failed to wait until network is gone")
		}
		return nil
	}
}
