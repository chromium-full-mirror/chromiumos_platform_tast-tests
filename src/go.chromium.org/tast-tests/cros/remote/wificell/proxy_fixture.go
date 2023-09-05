// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ProxyFixtBootToLoginScreen is a fixture name that will be registered to tast.
	ProxyFixtBootToLoginScreen = "proxyFixtBootToLoginScreen"
	// ProxyFixtBootToOOBEScreen is a fixture name that will be registered to tast.
	ProxyFixtBootToOOBEScreen = "proxyFixtBootToOOBEScreen"

	// ProxyFixtServiceDepsChromeBrowser is the service needed for proxyFixtureImpl.
	ProxyFixtServiceDepsChromeBrowser = "tast.cros.browser.ChromeService"
	// ProxyFixtServiceDepsProxySetting is the service needed for proxyFixtureImpl.
	ProxyFixtServiceDepsProxySetting = "tast.cros.network.ProxySettingService"

	// Boot the DUT then log in could takes up to 90 seconds.
	loginTimeout = 90 * time.Second
	// Connect to a wireless network could takes up to a minute.
	connectToNetworkTimeout = time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: ProxyFixtBootToLoginScreen,
		Desc: "The fixture is for proxy tests; boot the DUT to log-in screen, configure an AP, connect the AP and initiate the proxy-settings service client",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		SetUpTimeout:    15*time.Second + 2*loginTimeout, // Boot the DUT to sign-in screen requires log-in, log-out and then no-log-in.
		PreTestTimeout:  connectToNetworkTimeout,
		PostTestTimeout: 15 * time.Second,
		TearDownTimeout: 15 * time.Second,
		Impl:            &proxyFixtureImpl{isLoginScreen: true},
		ServiceDeps: []string{
			ProxyFixtServiceDepsProxySetting,
			ProxyFixtServiceDepsChromeBrowser,
			ShillServiceName,
		},
		Vars:   []string{"ui.signinProfileTestExtensionManifestKey", "router", "pcap", "routertype", "pcaptype"},
		Parent: FixtureID(TFFeaturesNone),
	})
	testing.AddFixture(&testing.Fixture{
		Name: ProxyFixtBootToOOBEScreen,
		Desc: "The fixture is for proxy tests; boot the DUT to log-in screen, configure an AP, connect the AP and initiate the proxy-settings service client",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		SetUpTimeout:    15 * time.Second,
		PreTestTimeout:  connectToNetworkTimeout,
		PostTestTimeout: 15 * time.Second,
		TearDownTimeout: 15 * time.Second,
		Impl:            &proxyFixtureImpl{},
		ServiceDeps: []string{
			ProxyFixtServiceDepsProxySetting,
			ProxyFixtServiceDepsChromeBrowser,
			ShillServiceName,
		},
		Vars:   []string{"ui.signinProfileTestExtensionManifestKey", "router", "pcap", "routertype", "pcaptype"},
		Parent: FixtureID(TFFeaturesNone),
	})
}

// DefaultProxyConfig returns a default proxy config for manual proxy configuration.
func DefaultProxyConfig(ssid string) *network.ProxyConfigs {
	return &network.ProxyConfigs{
		NetworkInfo:         &network.NetworkInfo{Value: &network.NetworkInfo_WifiSsid{WifiSsid: ssid}},
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            "localhost",
		HttpPort:            "123",
		HttpsHost:           "localhost",
		HttpsPort:           "456",
		SocksHost:           "socks5://localhost",
		SocksPort:           "8080",
	}
}

// DefaultProxyConfigForEthernet returns a default proxy config for manual proxy configuration on Ethernet network.
func DefaultProxyConfigForEthernet() *network.ProxyConfigs {
	return &network.ProxyConfigs{
		NetworkInfo:         &network.NetworkInfo{Value: &network.NetworkInfo_Ethernet{}},
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            "localhost",
		HttpPort:            "123",
		HttpsHost:           "localhost",
		HttpsPort:           "456",
		SocksHost:           "socks5://localhost",
		SocksPort:           "8080",
	}
}

// ProxyFixtureData provides a container for the data used by the proxyFixtureImpl fixture and tests that use this fixture.
type ProxyFixtureData struct {
	WifiTestFixture  *TestFixture
	AP               *APIface
	ProxySettingsSvc network.ProxySettingServiceClient
	CrSvc            ui.ChromeServiceClient
}

// Reboot reboots the DUT and manage/re-establishes the resources for proxy tests.
func (f *ProxyFixtureData) Reboot(ctx context.Context, manifestKey string) error {
	if _, err := f.CrSvc.Close(ctx, &emptypb.Empty{}); err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome service: ", err)
	}

	if err := f.WifiTestFixture.RebootDUT(ctx, DefaultDUT); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	// Re-initiate Chrome session.
	f.CrSvc = ui.NewChromeServiceClient(f.WifiTestFixture.DUTRPC(DefaultDUT).Conn)
	if _, err := f.CrSvc.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		SigninProfileTestExtensionId: manifestKey,
		KeepState:                    true,
	}); err != nil {
		return errors.Wrap(err, "failed to start chrome")
	}

	f.ProxySettingsSvc = network.NewProxySettingServiceClient(f.WifiTestFixture.DUTRPC(DefaultDUT).Conn)

	return nil
}

// proxyFixtureImpl implements testing.FixtureImpl, it sets up the DUT for proxy tests by
// start Chrome, configure an AP, connect to the AP and initiate the proxy-settings service client.
type proxyFixtureImpl struct {
	data          *ProxyFixtureData
	isLoginScreen bool
}

// SetUp sets up the DUT by boot the DUT to sign-in screen.
func (f *proxyFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	tf := s.ParentValue().(*TestFixture)
	if tf == nil {
		s.Fatal("Failed to obtain context for basic WiFi test")
	}

	crSvc := ui.NewChromeServiceClient(tf.DUTRPC(DefaultDUT).Conn)
	startChromeRequest := &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}

	// Create user pod and keep user profile to boot to login screen.
	if f.isLoginScreen {
		// At least one user profile must exist in order to logout and stay at the login screen.
		if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		// Properly close the connection before starting a new one.
		if _, err := crSvc.Close(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to close Chrome: ", err)
		}
		// Keep state to make sure an user profile exists at login screen.
		startChromeRequest.KeepState = true
	}

	// Create a new Chrome connection without logging in.
	if _, err := crSvc.New(ctx, startChromeRequest); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	f.data = &ProxyFixtureData{
		WifiTestFixture: tf,
		CrSvc:           crSvc,
	}
	return f.data
}

// Reset resets the environment between tests.
func (f *proxyFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

// PreTest performs the action before each proxy test.
// It initiates proxy settings service client, configures an AP and connects to the AP.
func (f *proxyFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.data.ProxySettingsSvc = network.NewProxySettingServiceClient(f.data.WifiTestFixture.DUTRPC(DefaultDUT).Conn)

	ap, err := f.data.WifiTestFixture.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	f.data.AP = ap

	if _, err := f.data.WifiTestFixture.ConnectWifiAPFromDUT(ctx, DefaultDUT, ap); err != nil {
		s.Fatal("Failed to connect to AP: ", err)
	}

	if err := f.data.WifiTestFixture.DUTWifiClient(DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true); err != nil {
		s.Fatal("Failed to wait for connected: ", err)
	}
}

// PostTest performs the action after each proxy test.
// It closes the proxy-settings service, note that the network AP shall be cleanup by parent fixture.
func (f *proxyFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.data.WifiTestFixture.CleanDisconnectDUTFromWifi(ctx, DefaultDUT); err != nil {
		s.Log("Failed to clean and disconnect network: ", err)
	}
}

// TearDown releases resources held open by the proxy fixture.
func (f *proxyFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if _, err := f.data.CrSvc.Close(ctx, &emptypb.Empty{}); err != nil {
		s.Log("Failed to close Chrome service: ", err)
	}
}
