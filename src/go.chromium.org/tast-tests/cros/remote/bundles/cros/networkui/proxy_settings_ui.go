// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProxySettingsUI,
		Desc: "Verify the UI for proxy settings",
		Contacts: []string{
			"cros-networking@google.com",
			"jiejiang@google.com",
		},
		BugComponent:   "b:156085", // ChromeOS > Platform > baseOS > Networking
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:           []string{"group:wificell_network_cellular"}, // Add "group:release-health", "release-health_wifi" after stabilized.
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ProxyFixtServiceDepsProxySetting,
			wificell.ProxyFixtServiceDepsChromeBrowser,
			wificell.ShillServiceName,
			"tast.cros.ui.ChromeUIService",
		},
		SoftwareDeps: []string{"chrome", "reboot"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "set_exception_and_same_for_all_proxy_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setExceptionAndSetUseSameForAllProxy,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "set_http_proxy_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setHTTPProxyOnly,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "set_secure_http_proxy_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setSecureHTTPProxyOnly,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "forget_then_set_socks_proxy_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  resetByForgettingNetwork,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "persistent_after_reboot_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  persistentAfterRebootLoginScreen,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "persistent_after_suspend_login_screen",
				Fixture: wificell.ProxyFixtBootToLoginScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  persistentAfterSuspendLoginScreen,
					loginMode: networkui.LoginMode_SignInScreen,
				},
			}, {
				Name:    "set_exception_and_same_for_all_proxy_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setExceptionAndSetUseSameForAllProxy,
					loginMode: networkui.LoginMode_OOBE,
				},
			}, {
				Name:    "set_http_proxy_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setHTTPProxyOnly,
					loginMode: networkui.LoginMode_OOBE,
				},
			}, {
				Name:    "set_secure_http_proxy_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  setSecureHTTPProxyOnly,
					loginMode: networkui.LoginMode_OOBE,
				},
			}, {
				Name:    "forget_then_set_socks_proxy_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  resetByForgettingNetwork,
					loginMode: networkui.LoginMode_OOBE,
				},
			}, {
				Name:    "persistent_after_reboot_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  persistentAfterRebootOOBE,
					loginMode: networkui.LoginMode_OOBE,
				},
			}, {
				Name:    "persistent_after_suspend_oobe",
				Fixture: wificell.ProxyFixtBootToOOBEScreen,
				Val: &proxySettingsUITestParam{
					testFunc:  persistentAfterSuspendOOBE,
					loginMode: networkui.LoginMode_OOBE,
				},
			},
		},
	})
}

type proxySettingsUITestParam struct {
	testFunc  func(context.Context, *proxySettingsUITestData) error
	loginMode networkui.LoginMode
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
	networkInfo *networkui.NetworkInfo
	manifestKey string
	loginMode   networkui.LoginMode
}

// ProxySettingsUI verifies the UI for proxy settings.
func ProxySettingsUI(ctx context.Context, s *testing.State) {
	proxyFixtureData := s.FixtValue().(*wificell.ProxyFixtureData)
	param := s.Param().(*proxySettingsUITestParam)

	networkInfo := &networkui.NetworkInfo{Value: &networkui.NetworkInfo_WifiSsid{WifiSsid: proxyFixtureData.AP.Config().SSID}}
	data := &proxySettingsUITestData{
		ProxyFixtureData: proxyFixtureData,
		networkInfo:      networkInfo,
		manifestKey:      s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		loginMode:        param.loginMode,
	}
	if err := param.testFunc(ctx, data); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}

// setExceptionAndSetUseSameForAllProxy verifies that proxy settings and exception can be saved also verifies the "Use same proxy for all protocols" toggle will be turned on when all proxy settings are the same.
func setExceptionAndSetUseSameForAllProxy(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
	}
	exceptionToBeSet := &networkui.SetExceptionRequest{
		NetworkInfo: data.networkInfo,
		Host:        "www.example.com",
		Action:      networkui.SetExceptionRequest_ADD,
		LoginMode:   data.loginMode,
	}
	expectedProxy := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            defaultProxyPort,
	}

	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   proxyToBeSet,
		LoginMode: data.loginMode,
	}); err != nil {
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
	removeExceptionReq.Action = networkui.SetExceptionRequest_REMOVE
	if _, err := proxySettingsSvc.SetException(ctx, removeExceptionReq); err != nil {
		return errors.Wrap(err, "failed to remove exception")
	}

	// The expected exception should be empty.
	if err := reopenProxySettingsAndVerifyException(ctx, data, nil); err != nil {
		return errors.Wrap(err, "failed to verify exception")
	}

	proxyToBeSet = &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            testProxyHost,
		HttpPort:            testProxyPort,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
		SocksHost:           testProxyHost,
		SocksPort:           testProxyPort,
	}
	expectedProxy = &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            testProxyPort,
	}
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   proxyToBeSet,
		LoginMode: data.loginMode,
	}); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}
	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// setHTTPProxyOnly verifies that the "Use same proxy for all protocols" toggle
// will be turned on when only HTTP proxy is set.
func setHTTPProxyOnly(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            testProxyHost,
		HttpPort:            testProxyPort,
	}
	expectedProxy := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		SameHost:            testProxyHost,
		SamePort:            testProxyPort,
	}
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   proxyToBeSet,
		LoginMode: data.loginMode,
	}); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// setSecureHTTPProxyOnly verifies that proxy can be saved when only Secure HTTP is set.
func setSecureHTTPProxyOnly(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
	}
	expectedProxy := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		HttpPort:            defaultProxyPort,
		HttpsHost:           testProxyHost,
		HttpsPort:           testProxyPort,
		SocksPort:           defaultSocketPort,
	}
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   proxyToBeSet,
		LoginMode: data.loginMode,
	}); err != nil {
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

	expectedProxy := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_DirectInternetConnection,
	}

	if err := reopenProxySettingsAndVerify(ctx, data, expectedProxy); err != nil {
		return errors.Wrap(err, "failed to verify")
	}

	proxySettingsSvc := data.ProxySettingsSvc
	proxyToBeSet := &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		SocksHost:           "socks",
		SocksPort:           defaultSocketPort,
	}

	expectHost := fmt.Sprintf("socks4://%s", proxyToBeSet.SocksHost)
	expectedProxy = &networkui.ProxyConfigs{
		NetworkInfo:         data.networkInfo,
		ProxyConnectionType: networkui.ProxyConnectionType_ManualProxyConfiguration,
		HttpHost:            expectHost,
		HttpPort:            defaultSocketPort,
		HttpsHost:           expectHost,
		HttpsPort:           defaultSocketPort,
		SocksHost:           proxyToBeSet.SocksHost,
		SocksPort:           defaultSocketPort,
	}
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   proxyToBeSet,
		LoginMode: data.loginMode,
	}); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}
	return reopenProxySettingsAndVerify(ctx, data, expectedProxy)
}

// setProxyAndReboot sets the proxy settings and reboots the DUT.
func setProxyAndReboot(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()),
		LoginMode: data.loginMode,
	}); err != nil {
		return err
	}

	proxyFixture := data.ProxyFixtureData
	if err := proxyFixture.Reboot(ctx, data.manifestKey); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	return nil
}

// persistentAfterRebootLoginScreen verifies that proxy settings is still remembered on login screen after rebooting.
func persistentAfterRebootLoginScreen(ctx context.Context, data *proxySettingsUITestData) error {
	if err := setProxyAndReboot(ctx, data); err != nil {
		return errors.Wrap(err, "failed to set proxy and reboot")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// persistentAfterRebootOOBE verifies that proxy settings is still remembered on OOBE screen after rebooting.
func persistentAfterRebootOOBE(ctx context.Context, data *proxySettingsUITestData) error {
	if err := setProxyAndReboot(ctx, data); err != nil {
		return errors.Wrap(err, "failed to set proxy and reboot")
	}

	crUISvc := ui.NewChromeUIServiceClient(data.WifiTestFixture.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to wait for OOBE screen to be ready")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// setProxyAndSuspend sets the proxy settings, suspends and resumes the DUT.
func setProxyAndSuspend(ctx context.Context, data *proxySettingsUITestData) error {
	proxySettingsSvc := data.ProxySettingsSvc
	if _, err := proxySettingsSvc.SetProxySettings(ctx, &networkui.SetProxySettingsRequest{
		Configs:   wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()),
		LoginMode: data.loginMode,
	}); err != nil {
		return errors.Wrap(err, "failed to set proxy")
	}

	if err := data.WifiTestFixture.DUTWifiClient(wificell.DefaultDUT).Suspend(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	if _, err := data.CrSvc.Reconnect(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reconnect to Chrome session")
	}
	return nil
}

// persistentAfterSuspendLoginScreen verifies that proxy settings is still remembered on login screen after suspending.
func persistentAfterSuspendLoginScreen(ctx context.Context, data *proxySettingsUITestData) error {
	if err := setProxyAndSuspend(ctx, data); err != nil {
		return errors.Wrap(err, "failed to set proxy, suspend and resume")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// persistentAfterSuspendOOBE verifies that proxy settings is still remembered on OOBE screen after suspending.
func persistentAfterSuspendOOBE(ctx context.Context, data *proxySettingsUITestData) error {
	if err := setProxyAndSuspend(ctx, data); err != nil {
		return errors.Wrap(err, "failed to set proxy, suspend and resume")
	}

	crUISvc := ui.NewChromeUIServiceClient(data.WifiTestFixture.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to wait for OOBE screen to be ready")
	}
	return reopenProxySettingsAndVerify(ctx, data, wificell.DefaultProxyConfig(data.networkInfo.GetWifiSsid()))
}

// reopenProxySettingsAndVerify reopens the proxy settings page and verifies the proxy settings is as expected.
func reopenProxySettingsAndVerify(ctx context.Context, data *proxySettingsUITestData, expectedProxyValue *networkui.ProxyConfigs) error {
	proxySettingsSvc := data.ProxySettingsSvc
	resp, err := proxySettingsSvc.FetchProxySettings(ctx, &networkui.FetchProxySettingsRequest{
		NetworkInfo: data.networkInfo,
		LoginMode:   data.loginMode,
	})
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
	resp, err := proxySettingsSvc.FetchException(ctx, &networkui.FetchExceptionRequest{
		NetworkInfo: data.networkInfo,
		LoginMode:   data.loginMode,
	})
	if err != nil {
		return errors.Wrap(err, "failed to fetch proxy settings")
	}

	if diff := cmp.Diff(resp.Exception, expectedException); diff != "" {
		return errors.Errorf("unexpected proxy values (-got +want): %s", diff)
	}
	return nil
}
