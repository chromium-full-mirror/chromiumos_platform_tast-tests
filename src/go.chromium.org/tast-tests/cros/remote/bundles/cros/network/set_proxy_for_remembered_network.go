// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	connectToKnownNetworkTimeout  = 30 * time.Second // Waiting for the known network to be connected could take a while.
	configureProxySettingsTimeout = 30 * time.Second // Configure proxy is a series of UI operations and it could take a while.
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SetProxyForRememberedNetwork,
		// This test requires fetch proxy configs from a web page (CrosNetworkConfig),
		// however, that page isn't available in lacros, only ash-Chrome is able to browse the page,
		// therefore the lacros variant is not needed.
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that proxy settings can be set for shared or non-shared network",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		VarDeps:        []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.wifi.WifiService",
			"tast.cros.browser.ChromeService",
			"tast.cros.network.ProxySettingService",
			"tast.cros.network.CrosNetworkConfigService",
			"tast.cros.ui.ChromeUIService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		// This test contains 3 sub-tests, all of them having 2 steps that consume the
		// majority of the time: configuring proxies and connecting to the network.
		// Each sub-test configures proxy for up to 2 times and connects to up to 1 other network.
		Timeout: 5*time.Minute + 3*(connectToKnownNetworkTimeout+2*configureProxySettingsTimeout),
	})
}

// SetProxyForRememberedNetwork verifies that proxy settings can be set for shared or non-shared network.
func SetProxyForRememberedNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	helper := newNetworksHelper(rpcClient, tf)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	defer tf.DeconfigAllAPs(cleanupCtx)
	for prefix, shared := range map[ssidPrefix]bool{
		sharedAndActive: true,
		shared:          true,
		nonShared:       false,
	} {
		if err := helper.configureNetwork(ctx, tf, rpcClient, prefix, shared); err != nil {
			s.Fatalf("Failed to configure the %q network: %v", prefix, err)
		}
	}

	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	// All added networks should be listed in known network.
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   helper.allNetworkSSID(),
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist},
	); err != nil {
		s.Fatal("Failed to ensure all added networks are listed in known networks: ", err)
	}

	// Connect to the network that the test expects to be active.
	if err := helper.connectTo(ctx, helper.aps[sharedAndActive].Config().SSID); err != nil {
		s.Fatal("Failed to ensure the active network is active: ", err)
	}

	for _, test := range []struct {
		description             string
		proxiesForSharedNetwork bool
	}{
		{
			description:             `user can set proxies for non-shared networks only when "Allow proxies for shared networks" toggle is off`,
			proxiesForSharedNetwork: false, // Not allow.
		}, {
			description:             `user can set proxies for shared and non-shared network when "Allow proxies for shared networks" toggle is on`,
			proxiesForSharedNetwork: true, // Allow.
		}, {
			description:             `user can not set proxies for shared networks when "Allow proxies for shared networks" toggle is back to off`,
			proxiesForSharedNetwork: false, // Not allow.
		},
	} {
		s.Run(ctx, test.description, func(ctx context.Context, s *testing.State) {
			if err := helper.toggleAllowProxiesForSharedNetworks(ctx, rpcClient, test.proxiesForSharedNetwork); err != nil {
				s.Fatal(`Failed to toggle the "Allow proxies for shared networks" option: `, err)
			}

			if err := checkWhetherProxiesAreConfigurable(ctx, rpcClient, helper, test.proxiesForSharedNetwork); err != nil {
				s.Fatal("Failed to check whether proxies are configurable: ", err)
			}
		})
	}
}

func checkWhetherProxiesAreConfigurable(ctx context.Context, rpcClient *rpc.Client, helper *networksHelper, allowProxies bool) error {
	proxy := wificell.DefaultProxyConfig("")
	differentProxy := wificell.DefaultProxyConfig("")
	// The trailing "9" is a random choice, not an important value as it's just meant to make this proxy different from `defaultProxy`.
	differentProxy.HttpPort += "9"

	if allowProxies {
		// Verify that user can set proxies for shared networks.
		ssid := helper.aps[shared].Config().SSID
		connectToNetworkBeforeVerify := false
		if err := setProxyAndVerify(ctx, rpcClient, helper, proxy, ssid, connectToNetworkBeforeVerify); err != nil {
			return errors.Wrap(err, "failed to verify proxy settings can be set correctly")
		}

		// Verify that user can set proxies for non-shared networks.
		ssid = helper.aps[nonShared].Config().SSID
		// This is a mimic of manual testing, do an end-to-end connection for this specific
		// network before checking the proxy to see if the proxy is correctly applied.
		connectToNetworkBeforeVerify = true

		if err := setProxyAndVerify(ctx, rpcClient, helper, differentProxy, ssid, connectToNetworkBeforeVerify); err != nil {
			return errors.Wrap(err, "failed to verify proxy settings can be set correctly")
		}

		// Restore the active network back to be active.
		if err := helper.connectTo(ctx, helper.aps[sharedAndActive].Config().SSID); err != nil {
			return errors.Wrap(err, "failed to ensure the active network is active")
		}
	} else {
		// User can set proxies for non-shared networks only and the proxy settings section should be restricted.
		if err := proxySettingsRestricted(ctx, rpcClient, []string{helper.aps[shared].Config().SSID}); err != nil {
			return errors.Wrap(err, "failed to verify the proxy settings section is restricted")
		}

		// Verify that user can set proxies for non-shared networks.
		ssid := helper.aps[nonShared].Config().SSID
		connectToNetworkBeforeVerify := false
		if err := setProxyAndVerify(ctx, rpcClient, helper, proxy, ssid, connectToNetworkBeforeVerify); err != nil {
			return errors.Wrap(err, "failed to verify proxy settings can be set correctly")
		}
	}
	return nil
}

func proxySettingsRestricted(ctx context.Context, rpcClient *rpc.Client, ssids []string) error {
	proxySvc := network.NewProxySettingServiceClient(rpcClient.Conn)

	for _, ssid := range ssids {
		if resp, err := proxySvc.IsProxySettingsRestricted(ctx, &network.IsProxySettingsRestrictedRequest{
			NetworkInfo: &network.NetworkInfo{
				Value: &network.NetworkInfo_WifiSsid{WifiSsid: ssid},
			},
			LoginMode: network.LoginMode_LoggedIn,
		}); err != nil {
			errors.Wrapf(err, "failed to check if proxy settings of network %q is restricted", ssid)
		} else if !resp.Value {
			return errors.New("proxy settings is not restricted")
		}
	}

	return nil
}

// setProxyAndVerify sets up the proxy settings via the ossettings network detail page,
// and checks if the setup is successful by verifying CrosNetworkConfig.
func setProxyAndVerify(ctx context.Context, rpcClient *rpc.Client, helper *networksHelper, proxyConfig *network.ProxyConfigs, ssid string, connectToNetworkBeforeVerify bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	proxySvc := network.NewProxySettingServiceClient(rpcClient.Conn)
	defer proxySvc.ResetConnectionType(cleanupCtx, &network.ResetConnectionTypeRequest{
		NetworkInfo: &network.NetworkInfo{
			Value: &network.NetworkInfo_WifiSsid{WifiSsid: ssid},
		},
		LoginMode: network.LoginMode_LoggedIn,
	})

	proxyConfig.NetworkInfo.Value = &network.NetworkInfo_WifiSsid{WifiSsid: ssid}
	if _, err := proxySvc.SetProxySettings(ctx, &network.SetProxySettingsRequest{
		Configs:   proxyConfig,
		LoginMode: network.LoginMode_LoggedIn,
	}); err != nil {
		return errors.Wrap(err, "failed to set up proxy settings")
	}

	if connectToNetworkBeforeVerify {
		if err := helper.connectTo(ctx, ssid); err != nil {
			return errors.Wrapf(err, "failed to connect to %q", ssid)
		}
	}

	resp, err := proxySvc.FetchProxySettings(ctx, &network.FetchProxySettingsRequest{
		NetworkInfo: &network.NetworkInfo{
			Value: &network.NetworkInfo_WifiSsid{WifiSsid: ssid},
		},
		FetchSource: network.FetchProxySettingsRequest_CrosNetworkConfig,
		LoginMode:   network.LoginMode_LoggedIn,
	})
	if err != nil {
		return errors.Wrap(err, "failed to fetch proxy settings")
	}

	if diff := cmp.Diff(resp, proxyConfig, protocmp.Transform()); diff != "" {
		return errors.Errorf("unexpected proxy values (-want +got): %s", diff)
	}

	return nil
}

type ssidPrefix string

const (
	// The main test subject, a network that is not shared (and also not active).
	nonShared ssidPrefix = "Network_O_NonShared_"
	// The main test subject, a network that is shared (and also not active).
	shared ssidPrefix = "Network_N_Shared_"

	// The network that will and should remain as active so that other test subjects are
	// not active (while setting up the proxy).
	// This network will also be used for toggling the "Allow proxies for shared networks".
	sharedAndActive ssidPrefix = "Network_M_SharedAndActive_"
)

type networksHelper struct {
	aps       map[ssidPrefix]*wificell.APIface
	rpcClient *rpc.Client
	tf        *wificell.TestFixture
}

func newNetworksHelper(rpcClient *rpc.Client, tf *wificell.TestFixture) *networksHelper {
	return &networksHelper{
		aps:       make(map[ssidPrefix]*wificell.APIface),
		rpcClient: rpcClient,
		tf:        tf,
	}
}

func (helper *networksHelper) allNetworkSSID() []string {
	return []string{helper.aps[sharedAndActive].Config().SSID, helper.aps[shared].Config().SSID, helper.aps[nonShared].Config().SSID}
}

func (helper *networksHelper) configureNetwork(ctx context.Context, tf *wificell.TestFixture, rpcClient *rpc.Client, prefix ssidPrefix, shared bool) error {
	const testPass string = "chromeos"

	securityConfig := wpa.NewConfigFactory(testPass, wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP, wpa.CipherTKIP))
	apOpts := []hostapd.Option{hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211g)}

	netConfigSvc := network.NewCrosNetworkConfigServiceClient(rpcClient.Conn)

	apOpts = append(apOpts, hostapd.SSID(hostapd.RandomSSID(string(prefix))))

	ap, err := tf.ConfigureAP(ctx, apOpts, securityConfig)
	if err != nil {
		return errors.Wrap(err, "failed to configure AP")
	}

	helper.aps[prefix] = ap

	wifiConfigProperties := &network.WiFiConfigProperties{
		Ssid:           ap.Config().SSID,
		HiddenSsidMode: network.HiddenSSIDMode_Disabled,
		Security: &network.WiFiConfigProperties_Psk{
			Psk: testPass,
		},
	}
	networkTypeConfigProperties := &network.NetworkTypeConfigProperties{
		ConfigProperties: &network.NetworkTypeConfigProperties_WifiConfigProperties{
			WifiConfigProperties: wifiConfigProperties,
		},
	}
	if _, err := netConfigSvc.ConfigureNetwork(ctx, &network.ConfigureNetworkRequest{
		Shared:           shared,
		ConfigProperties: networkTypeConfigProperties,
	}); err != nil {
		return err
	}

	return nil
}

func (helper *networksHelper) connectTo(ctx context.Context, ssid string) error {
	wifiSvc := wifi.NewWifiServiceClient(helper.rpcClient.Conn)
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_Connect,
	}); err != nil {
		return errors.Wrapf(err, "failed to connect to %q", ssid)
	}
	return helper.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ssid, true /* expectedValue */)
}

func (helper *networksHelper) toggleAllowProxiesForSharedNetworks(ctx context.Context, rpcClient *rpc.Client, isAllow bool) error {
	// The "Allow proxies for shared networks" is a shared option across all shared networks,
	// toggling it in one network will allow/disallow proxies for all shared networks.
	ssid := helper.aps[sharedAndActive].Config().SSID

	proxySvc := network.NewProxySettingServiceClient(rpcClient.Conn)
	if _, err := proxySvc.AllowProxiesForSharedNetwork(ctx, &network.AllowProxiesForSharedNetworkRequest{
		Allow: isAllow,
		NetworkInfo: &network.NetworkInfo{
			Value: &network.NetworkInfo_WifiSsid{WifiSsid: ssid},
		},
		LoginMode: network.LoginMode_LoggedIn,
	}); err != nil {
		return errors.Wrap(err, `failed to toggle "Allow proxies for shared networks"`)
	}

	return nil
}
