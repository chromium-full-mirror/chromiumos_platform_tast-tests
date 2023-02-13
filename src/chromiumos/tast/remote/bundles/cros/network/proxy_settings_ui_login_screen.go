// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/errors"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/services/cros/network"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProxySettingsUILoginScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the UI for proxy settings works on Login screen",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"cj.tsai@cienet.com",
			"cienet-development@googlegroups.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:mainline", "informational"},
		ServiceDeps: []string{
			wificell.ProxyFixtServiceDepsProxySetting,
			wificell.ProxyFixtServiceDepsChromeBrowser,
			wificell.TFServiceName,
		},
		SoftwareDeps: []string{"chrome", "reboot"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      wificell.ProxyFixtBootToSigninScreen,
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "set_exception_and_same_for_all_proxy",
				Val:  setExceptionAndSetUseSameForAllProxy,
			}, {
				Name: "set_http_proxy",
				Val:  setHTTPProxyOnly,
			}, {
				Name: "set_secure_http_proxy",
				Val:  setSecureHTTPProxyOnly,
			}, {
				Name: "forget_then_set_socks_proxy",
				Val:  resetByForgettingNetwork,
			}, {
				Name: "persistent_after_reboot",
				Val:  persistentAfterReboot,
			}, {
				Name: "persistent_after_suspend",
				Val:  persistentAfterSuspend,
			},
		},
	})
}

const (
	testProxyHost     = "localhost"
	testProxyPort     = "123"
	defaultProxyPort  = "80"
	defaultSocketPort = "1080"
)

// proxySettingsUITestData stores the resources for the proxy Settings UI tests.
type proxySettingsUITestData struct {
	*wificell.ProxyFixtureData
	networkInfo *network.NetworkInfo
	manifestKey string
}

// ProxySettingsUILoginScreen verifies the UI for proxy settings works on login screen.
func ProxySettingsUILoginScreen(ctx context.Context, s *testing.State) {
	proxyFixtureData := s.FixtValue().(*wificell.ProxyFixtureData)

	networkInfo := &network.NetworkInfo{Value: &network.NetworkInfo_WifiSsid{WifiSsid: proxyFixtureData.AP.Config().SSID}}
	data := &proxySettingsUITestData{
		ProxyFixtureData: proxyFixtureData,
		networkInfo:      networkInfo,
		manifestKey:      s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}
	testFunc := s.Param().(func(context.Context, *proxySettingsUITestData) error)
	if err := testFunc(ctx, data); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}

// setExceptionAndSetUseSameForAllProxy verifies that proxy settings and exception can be saved also verifies the "Use same proxy for all protocols" toggle will be turned on when all proxy settings are the same.
func setExceptionAndSetUseSameForAllProxy(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
	}
	exceptionToBeSet := &network.SetExceptionRequest{
		NetworkInfo: data.networkInfo,
		Host:        "www.example.com",
		Action:      network.SetExceptionRequest_ADD,
	}
	expectedProxy := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            defaultProxyPort,
	}

	if _, err := proxySettingsSvc.Setup(ctx, proxyToBeSet); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	if err := reopenProxySettingsAndVerify(ctx, data, expectedProxy); err != nil {
		return errors.Wrap(err, "failed to verify")
	}

	if _, err := proxySettingsSvc.SetException(ctx, exceptionToBeSet); err != nil {
		return errors.Wrap(err, "failed to set exception")
	}

	if err := reopenProxySettingsAndVerifyException(ctx, data, []string{exceptionToBeSet.Host}); err != nil {
		return errors.Wrap(err, "failed to verify exception")
	}

	removeExceptionReq := exceptionToBeSet
	removeExceptionReq.Action = network.SetExceptionRequest_REMOVE
	if _, err := proxySettingsSvc.SetException(ctx, removeExceptionReq); err != nil {
		return errors.Wrap(err, "failed to remove exception")
	}

	// The expected exception should be empty.
	if err := reopenProxySettingsAndVerifyException(ctx, data, nil); err != nil {
		return errors.Wrap(err, "failed to verify exception")
	}

	proxyToBeSet = &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            testProxyHost,
		HttpPort:            testProxyPort,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
		SocksHost:           testProxyHost,
		SocksPort:           testProxyPort,
	}
	expectedProxy = &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            testProxyPort,
	}
	if _, err := proxySettingsSvc.Setup(ctx, proxyToBeSet); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}
	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// setHTTPProxyOnly verifies that the "Use same proxy for all protocols" toggle
// will be turned on when only HTTP proxy is set.
func setHTTPProxyOnly(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            testProxyHost,
		HttpPort:            testProxyPort,
	}
	expectedProxy := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            testProxyPort,
	}
	if _, err := proxySettingsSvc.Setup(ctx, proxyToBeSet); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// setSecureHTTPProxyOnly verifies that proxy can be saved when only Secure HTTP is set.
func setSecureHTTPProxyOnly(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
	}
	expectedProxy := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpPort:            defaultProxyPort,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
		SocksPort:           defaultSocketPort,
	}
	if _, err := proxySettingsSvc.Setup(ctx, proxyToBeSet); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// resetByForgettingNetwork verifies that proxy settings will be reverted to default value after forgetting the network then
// verifies that other proxies will start with "socks4://" when only Socks is set.
func resetByForgettingNetwork(ctx context.Context, data *proxySettingsUITestData) error {
	tf := data.WifiTestFixture

	// Using CleanDisconnectDUTFromWifi to perform forget network settings.
	if err := tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
		return errors.Wrap(err, "failed to forget WiFi")
	}

	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, data.networkInfo.GetWifiSsid(), false); err != nil {
		return errors.Wrap(err, "the network is still connected")
	}

	if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, data.AP); err != nil {
		return err
	}
	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, data.networkInfo.GetWifiSsid(), true); err != nil {
		return errors.Wrap(err, "failed to wait for connected")
	}

	expectedProxy := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_DirectInternetConnection,
	}

	if err := reopenProxySettingsAndVerify(ctx, data, expectedProxy); err != nil {
		return errors.Wrap(err, "failed to verify")
	}

	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		SocksHost:           "socks",
		SocksPort:           defaultSocketPort,
	}

	expectHost := fmt.Sprintf("socks4://%s", proxyToBeSet.SocksHost)
	expectedProxy = &network.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: network.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            expectHost,
		HttpPort:            defaultSocketPort,
		HttpsHost:           expectHost,
		HttpsPort:           defaultSocketPort,
		SocksHost:           proxyToBeSet.SocksHost,
		SocksPort:           defaultSocketPort,
	}
	if _, err := proxySettingsSvc.Setup(ctx, proxyToBeSet); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}
	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// persistentAfterReboot verifies that proxy settings is still remembered after rebooting.
func persistentAfterReboot(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.Setup(ctx, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid())); err != nil {
		return err
	}

	proxyFixture := data.ProxyFixtureData
	if err := proxyFixture.Reboot(ctx, data.manifestKey); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// persistentAfterSuspend verifies that proxy settings is still remembered after suspending.
func persistentAfterSuspend(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.Setup(ctx, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid())); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	if err := data.WifiTestFixture.DUTWifiClient(wificell.DefaultDUT).Suspend(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	if _, err := data.CrSvc.Reconnect(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reconnect to Chrome session")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// reopenProxySettingsAndVerify reopens the proxy settings page and verifies the proxy settings is as expected.
func reopenProxySettingsAndVerify(ctx context.Context, data *proxySettingsUITestData, expectedProxyValue *network.ProxyConfigs) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.Close(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to close the proxy settings service")
	}

	if _, err := proxySettingsSvc.Initialize(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start the proxy settings service")
	}

	resp, err := proxySettingsSvc.FetchProxySettings(ctx, &network.FetchProxySettingsRequest{NetworkInfo: data.networkInfo})
	if err != nil {
		return errors.Wrap(err, "failed to fetch proxy settings")
	}

	if diff := cmp.Diff(resp, expectedProxyValue, protocmp.Transform()); diff != "" {
		return errors.Errorf("unexpected proxy values (-got +want): %s", diff)
	}
	return nil
}

// reopenProxySettingsAndVerifyException reopens the proxy settings page and verifies the exception is as expected.
func reopenProxySettingsAndVerifyException(ctx context.Context, data *proxySettingsUITestData, expectedException []string) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.Close(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to close the proxy settings service")
	}

	if _, err := proxySettingsSvc.Initialize(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start the proxy settings service")
	}

	resp, err := proxySettingsSvc.FetchException(ctx, &network.FetchExceptionRequest{NetworkInfo: data.networkInfo})
	if err != nil {
		return errors.Wrap(err, "failed to fetch proxy settings")
	}

	if diff := cmp.Diff(resp.Exception, expectedException); diff != "" {
		return errors.Errorf("unexpected proxy values (-got +want): %s", diff)
	}
	return nil
}
